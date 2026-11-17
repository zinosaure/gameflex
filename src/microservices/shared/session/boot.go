package session

// LE CHILD DIT LUI-MÊME QU'IL EST PRÊT (#751) — le kernel ne sonde plus son `/status` pour le savoir.
//
// Le kernel lançait un conteneur, puis lui redemandait `/status` toutes les secondes, jusqu'à quatre-vingt-dix,
// pour savoir quand il répondait, et, sur streamex, où en était le déballage d'un jeu (`#262`). C'est un sondage, et
// la règle du porteur n'en admet aucun (« Tous les sondages se font par websocket. Pas d'exception », 2026-11-16 ;
// CODE_CONVENTIONS.md, « Pas de sondage »). Celui qui sait qu'il est prêt, c'est le child : il le dit, une fois, quand
// son port écoute. Le déballage de streamex est rapporté de la même façon, à chaque relevé de son avancée.
//
// La route est `POST <kernel>/internal/sessions/<id>/boot`, sous le secret de la partie, comme le miroir.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Boot — ce que le child dit de son démarrage : « booting » avec l'avancée d'un déballage, ou « ready ».
type Boot struct {
	State     string         `json:"state"`
	Unpacking *BootUnpacking `json:"unpacking,omitempty"`
}

// BootUnpacking — un déballage en cours : octets écrits sur ce que pèse l'archive.
type BootUnpacking struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

// BootReady — le child écoute : le kernel peut lui parler.
var BootReady = Boot{State: "ready"}

var bootClient = &http.Client{Timeout: 5 * time.Second}

// ReportBoot pousse un état de démarrage au kernel. Sans adresse ni secret, rien ne part — une erreur le dit.
func ReportBoot(kernelURL, secret, sessionID string, boot Boot) error {
	if kernelURL == "" || secret == "" || sessionID == "" {
		return fmt.Errorf("boot report: no kernel url, secret or session id")
	}
	body, err := json.Marshal(boot)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost,
		strings.TrimSuffix(kernelURL, "/")+"/internal/sessions/"+sessionID+"/boot", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", secret)

	response, err := bootClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("boot report: kernel answered %s", response.Status)
	}

	return nil
}

// AnnounceReady pousse BootReady, et le redit au plus trois fois si l'envoi échoue : perdu, il laisserait le lancement
// attendre jusqu'à son échéance un child qui écoute déjà. Un refus du kernel (secret faux) ne se redit pas.
func AnnounceReady(kernelURL, secret, sessionID string) error {
	err := ReportBoot(kernelURL, secret, sessionID, BootReady)
	for _, wait := range []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second} {
		if err == nil || strings.Contains(err.Error(), "kernel answered") {
			break
		}
		time.Sleep(wait)
		err = ReportBoot(kernelURL, secret, sessionID, BootReady)
	}

	return err
}
