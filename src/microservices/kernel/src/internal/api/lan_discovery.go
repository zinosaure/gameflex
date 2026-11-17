package api

// LE RÉSEAU LOCAL, PAR LA PASSERELLE (chantier 60). Le kernel est sur le
// bridge `gameflex_default`, que le multicast de mDNS ne traverse pas ; la
// passerelle WebRTC est en réseau hôte, et il la joint déjà
// (GatewayControlURL, sous le secret interne). Elle annonce et cherche pour
// lui (gatewaywebrtc, internal/api/lan.go) ; la décision reste ici :
//
//   - L'ANNONCE suit le mode serveur, et lui seul : la trame `pairing` dit son
//     échéance ; à chaque ouverture, prolongation et fermeture, le kernel
//     appelle PUT ou DELETE /internal/lan/announce. Il rejoue l'annonce à la
//     première trame de chaque connexion au service des comptes : l'un des deux
//     a pu être relancé ;
//   - LA RECHERCHE est le sujet `discovery` : le socket /internal/lan/browse
//     s'ouvre avec le premier abonné et se ferme avec le dernier.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"gameflex/facade"
)

// lanCallTimeout — la passerelle est sur la même machine.
const lanCallTimeout = 2 * time.Second

// lanLink — l'adresse qu'on annonce : celle d'edge, sur l'adresse de l'hôte.
func (server *Server) lanLink() string {
	if server.config.PublicHost == "" || server.config.EdgePort == "" {
		return ""
	}

	return "http://" + net.JoinHostPort(server.config.PublicHost, server.config.EdgePort)
}

// followServerMode — la trame dit le mode serveur ouvert jusqu'à `mode.OpenUntil`,
// ou fermé (nil) : l'annonce suit. `first` la rejoue même inchangée.
func (server *Server) followServerMode(mode *facade.PairingServerMode, first bool) {
	desired := ""
	if mode != nil {
		desired = mode.OpenUntil
	}
	server.lanMu.Lock()
	previous := server.lanAnnounced
	if desired == previous && !first {
		server.lanMu.Unlock()

		return
	}
	server.lanAnnounced = desired
	server.lanMu.Unlock()

	if desired == "" {
		if previous != "" || first {
			server.callLAN(http.MethodDelete, nil)
		}

		return
	}
	until, err := time.Parse(time.RFC3339, desired)
	if err != nil {
		return
	}
	ttl := int(time.Until(until).Round(time.Second) / time.Second)
	link := server.lanLink()
	if ttl < 1 || link == "" {
		return
	}
	name := server.federationHostname()
	port, _ := strconv.Atoi(server.config.EdgePort)
	server.callLAN(http.MethodPut, map[string]any{
		"instance": name + " (" + server.config.EdgePort + ")", "name": name, "link": link,
		"version": os.Getenv("GAMEFLEX_VERSION"), "address": server.config.PublicHost,
		"port": port, "ttl": ttl,
	})
}

// callLAN — PUT ou DELETE /internal/lan/announce chez la passerelle.
func (server *Server) callLAN(method string, body any) {
	base := strings.TrimRight(server.config.GatewayControlURL, "/")
	if base == "" || server.config.InternalSecret == "" {
		return
	}
	payload, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(context.Background(), lanCallTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, base+"/internal/lan/announce", bytes.NewReader(payload))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		slog.Warn("lan: the gateway does not answer", "method", method, "error", err)

		return
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		slog.Warn("lan: the gateway refused", "method", method, "status", response.StatusCode)

		return
	}
	slog.Info("lan: announcement", "method", method)
}

// lanRetryLimit — une passerelle relancée pendant une recherche : on la
// rejoint de nouveau, une seconde puis le double, jusqu'à trente.
const lanRetryLimit = 30 * time.Second

// searchLAN — la recherche du sujet `discovery`, par le socket de la
// passerelle, jusqu'à la fin de ctx.
func (server *Server) searchLAN(ctx context.Context, found func([]facade.DiscoveredServer)) func() {
	base := strings.TrimRight(server.config.GatewayControlURL, "/")
	if base == "" || server.config.InternalSecret == "" {
		return nil
	}
	target := "ws" + strings.TrimPrefix(base, "http") + "/internal/lan/browse"
	asks := make(chan struct{}, 1)

	go func() {
		wait := time.Second
		for ctx.Err() == nil {
			if server.browseOnce(ctx, target, found, asks) {
				wait = time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if wait *= 2; wait > lanRetryLimit {
				wait = lanRetryLimit
			}
		}
	}()

	return func() {
		select {
		case asks <- struct{}{}:
		default:
		}
	}
}

// browseOnce — une connexion au socket de la passerelle, jusqu'à sa fin. Vrai
// si une liste est arrivée.
func (server *Server) browseOnce(ctx context.Context, target string, found func([]facade.DiscoveredServer), asks <-chan struct{}) bool {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target, http.Header{"X-Internal-Secret": {server.config.InternalSecret}})
	if err != nil {
		slog.Warn("lan: cannot browse through the gateway", "error", err)

		return false
	}
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-ctx.Done():
				conn.Close()

				return
			case <-done:
				return
			case <-asks:
				_ = conn.WriteJSON(map[string]string{"type": "ask"})
			}
		}
	}()

	received := false
	for {
		var message struct {
			Servers []facade.DiscoveredServer `json:"servers"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return received
		}
		received = true
		found(message.Servers)
	}
}

// facadeDiscoverySearch — `{type:"discovery.search", id}` : reposer la
// question du réseau local, une fois. 409 sans recherche ouverte (aucune page
// abonnée au sujet `discovery`).
func (server *Server) facadeDiscoverySearch(conn *facade.Conn, request facade.Request) {
	if !server.discovery.Ask() {
		conn.Reply(request.ID, http.StatusConflict, facade.ErrorBody("not_searching", "subscribe to discovery first"))

		return
	}
	conn.Reply(request.ID, http.StatusOK, facade.OKBody(map[string]bool{"asked": true}))
}
