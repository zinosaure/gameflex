package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// LA FICHE D'UN JEU SWITCH, SUR LE SERVEUR : les émulateurs de ses images,
// présents (docker.StreamexCores) — Eden puis Citron (chantier 26, #438).
// Sans ces entrées, la fiche ne grisait rien.
func TestTheServerOffersTheEmulatorsOfItsImages(t *testing.T) {
	installed := map[string]bool{}
	names := []string{}
	for _, entry := range (&Server{}).streamexCoreEntries(context.Background(), "switch") {
		installed[entry.Name] = entry.Installed
		names = append(names, entry.Name)
	}
	if len(names) != 2 || names[0] != "Eden (streamex)" || !installed["Citron (streamex)"] || !installed["Eden (streamex)"] {
		t.Fatalf("switch : %v %v, attendu Eden puis Citron, présents", names, installed)
	}
	if entries := (&Server{}).streamexCoreEntries(context.Background(), "nes"); len(entries) != 0 {
		t.Fatalf("une console rétro n'a pas d'émulateur en conteneur : %v", entries)
	}
}

// UN ÉMULATEUR QU'AUCUNE IMAGE N'A SE REFUSE au lieu de partir sur Eden —
// c'est le cas d'un pair, dont la fiche ne juge pas nos émulateurs. Le refus
// vient avant toute session : rien à défaire.
func TestTheServerRefusesAnEmulatorItsImageLacks(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"platform":"streamex","game":"Mini Metro","console_id":"switch","core":"Yuzu (streamex)"}`)
	server.handleLaunch(recorder, httptest.NewRequest(http.MethodPost, "/launch", body))
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "Eden (streamex), Citron (streamex)") {
		t.Fatalf("un émulateur absent de l'image : %d %s", recorder.Code, recorder.Body.String())
	}
}
