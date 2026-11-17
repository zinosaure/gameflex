package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"gameflex/kernel/internal/auth"
)

// LA SRAM D'UN JEU SE LIT POUR LE COMPTE DU JETON, et pour lui seul : celle de
// gino existe, celle d'un autre compte pour le même jeu ne compte pas, et un
// jeu sans SRAM répond « rien » sans erreur.
func TestTheSaveRAMIsReadForTheTokensAccount(t *testing.T) {
	server, root := savesTestServer(t)
	handler := auth.Middleware(server.config.JWTSecret, server.handleRetroSaveRAM)
	sram := filepath.Join(root, "saves", "gino", "sram", "nes", "metroid-ref.srm")
	if err := os.MkdirAll(filepath.Dir(sram), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sram, []byte("partie"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func(user, query string) (int, retroSaveRAMInfo) {
		t.Helper()
		answer := savesRequest(t, handler, http.MethodGet, "/api/v1/retrogaming/savestate/sram?"+query, testToken(t, auth.Claims{Username: user}), nil)
		// L'enveloppe du kernel : `{success, data}` (writeJSON).
		var envelope struct {
			Data retroSaveRAMInfo `json:"data"`
		}
		_ = json.Unmarshal(answer.Body.Bytes(), &envelope)

		return answer.Code, envelope.Data
	}

	if code, info := read("gino", "consoleId=nes&reference=metroid-ref"); code != http.StatusOK || !info.Exists || info.Timestamp == nil {
		t.Fatalf("la SRAM de gino : %d %+v", code, info)
	}
	if code, info := read("alice", "consoleId=nes&reference=metroid-ref"); code != http.StatusOK || info.Exists {
		t.Fatalf("la SRAM d'un autre compte : %d %+v", code, info)
	}
	if code, info := read("gino", "consoleId=nes&reference=zelda-ref"); code != http.StatusOK || info.Exists || info.Timestamp != nil {
		t.Fatalf("un jeu sans SRAM : %d %+v", code, info)
	}
	if code, _ := read("gino", "consoleId=nes"); code != http.StatusBadRequest {
		t.Fatalf("sans référence : %d", code)
	}
	if code, _ := read("gino", "consoleId=..&reference=metroid-ref"); code != http.StatusBadRequest {
		t.Fatalf("une console qui sort du dossier : %d", code)
	}
}
