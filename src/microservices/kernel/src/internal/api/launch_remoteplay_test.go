package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gameflex/kernel/internal/config"
)

// L'ENVELOPPE DU RUNTIME REMOTE-PLAY (#195, régression trouvée le 2026-09-19).
//
// `writeJSON` du service remote-play enveloppe TOUJOURS sa charge utile
// (`{"success":…, "data":…}`) alors que `writeError` reste plat
// (`{"success":false,"errorMessage":…}`). `askRemoteplayControl` décodait à
// plat dans les deux cas. Les ERREURS arrivaient donc correctement — ce qui a
// masqué la panne pendant toutes mes sondes, qui ne sollicitaient qu'un
// `hostId` inexistant, donc un échec — et les SUCCÈS repartaient vides, sans
// que Go ne signale quoi que ce soit : un champ absent n'est pas une erreur de
// décodage.
//
// Ce que ça donnait en vrai : la porte résolvait l'appareil (200 dans le
// journal du runtime), recevait un `hostType` vide, et `launchRemoteplay`
// refusait avec « hostId and hostType are required » — un `writeError` SANS
// ligne de journal. La poignée de main n'était jamais demandée et la page
// n'affichait qu'un « Failed to create the session » sans cause visible.
//
// Le bon motif existait déjà à vingt lignes de là : `remoteplayDevices`
// (federation_devices.go) décode `envelope.Data`. Il n'a pas été repris.
func TestAskRemoteplayControlDecodesTheRuntimeEnvelope(t *testing.T) {
	const (
		wantHostType = "PS5"
		testSecret   = "secret-de-test"
	)

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Internal-Secret"); got != testSecret {
			t.Errorf("secret interne absent ou faux : %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		// La forme EXACTE relevée sur le runtime le 2026-09-19, et non une
		// forme plausible : c'est le décalage entre les deux qui est le sujet.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"deviceId": "id_60aac685aa380173dbdb4cc2fa7dc2c0",
				"hostId":   "192.168.1.12",
				"hostName": "PS5-896",
				"hostType": wantHostType,
			},
		})
	}))
	defer runtime.Close()

	server := &Server{config: config.Config{
		RemoteplayControlURL: runtime.URL,
		InternalSecret:       testSecret,
	}}

	var device struct {
		DeviceID string `json:"deviceId"`
		HostName string `json:"hostName"`
		HostType string `json:"hostType"`
	}
	if err := server.askRemoteplayControl("/internal/remoteplay/devices/resolve",
		map[string]string{"hostId": "192.168.1.12"}, &device); err != nil {
		t.Fatalf("resolve a échoué : %v", err)
	}

	// C'EST `hostType` QUI DÉCIDE. Sans lui, `launchRemoteplay` refuse avant
	// tout lancement — c'est très exactement la panne que ce test garde.
	if device.HostType != wantHostType {
		t.Errorf("hostType = %q, attendu %q — l'enveloppe n'est pas décodée", device.HostType, wantHostType)
	}
	if device.DeviceID == "" {
		t.Error("deviceId vide — la poignée de main n'aurait aucun appareil à joindre")
	}
	if device.HostName == "" {
		t.Error("hostName vide — le conteneur serait nommé sans sa console")
	}
}

// L'ERREUR, ELLE, EST PLATE — ET DOIT LE RESTER.
//
// Le même client lit `errorMessage` au premier niveau, et ce chemin-là
// fonctionnait : c'est par lui que « device not found for hostId: … » remonte
// jusqu'à la page. Ce test est le témoin de contrôle de la correction
// ci-dessus : sans lui, déplacer le décodage vers `data` pourrait casser en
// silence le seul chemin qui marchait.
func TestAskRemoteplayControlSurfacesTheFlatErrorMessage(t *testing.T) {
	const wantMessage = "device not found for hostId: PROBE-INEXISTANT"

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":      false,
			"errorMessage": wantMessage,
		})
	}))
	defer runtime.Close()

	server := &Server{config: config.Config{
		RemoteplayControlURL: runtime.URL,
		InternalSecret:       "secret-de-test",
	}}

	var ignored struct{}
	err := server.askRemoteplayControl("/internal/remoteplay/devices/resolve",
		map[string]string{"hostId": "PROBE-INEXISTANT"}, &ignored)
	if err == nil {
		t.Fatal("un 404 doit produire une erreur")
	}
	if err.Error() != wantMessage {
		t.Errorf("message = %q, attendu %q", err.Error(), wantMessage)
	}
}

// UNE RÉPONSE SANS CORPS RESTE LÉGITIME.
//
// La poignée de main (`/internal/remoteplay/handshake`) passe `out = nil` :
// elle n'a rien à lire au-delà du code HTTP. C'est pour ça qu'elle n'a jamais
// été touchée par le défaut d'enveloppe — et c'est aussi pour ça qu'il faut
// vérifier que la correction ne la casse pas en voulant déballer un `data`
// que personne n'a demandé.
func TestAskRemoteplayControlAcceptsAnIgnoredBody(t *testing.T) {
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": nil})
	}))
	defer runtime.Close()

	server := &Server{config: config.Config{
		RemoteplayControlURL: runtime.URL,
		InternalSecret:       "secret-de-test",
	}}

	if err := server.askRemoteplayControl("/internal/remoteplay/handshake",
		map[string]any{"deviceId": "id_x", "port": 9601}, nil); err != nil {
		t.Fatalf("la poignée de main ne lit aucun corps, elle ne doit pas échouer : %v", err)
	}
}
