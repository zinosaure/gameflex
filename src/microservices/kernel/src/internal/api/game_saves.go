package api

import (
	"net/http"
	"os"
	"time"

	"gameflex/kernel/internal/auth"
	"gameflex/storage"
)

// handleRetroSaveRAM — GET /api/v1/retrogaming/savestate/sram?consoleId=&reference=
//
// LA SAUVEGARDE QUE LE JEU ÉCRIT LUI-MÊME (sa cartouche, sa carte mémoire), pour
// ce compte : existe-t-elle, et de quand. Le launcher en a besoin pour dire
// « Continuer » d'un jeu qui n'a aucun état mais une partie en cours (chantier
// 51, #660) : les états se lisent déjà (`/savestate/user`), la SRAM nulle part.
// Elle se range comme le runtime l'écrit (storage.SaveRAMPath, la même clé :
// la référence du jeu) ; `?source=` lit une copie, comme les autres routes.
func (server *Server) handleRetroSaveRAM(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	// Du jeton, jamais de la requête — voir la doc de retroRouter.
	if consoleID == "" || reference == "" || auth.Username(r.Context()) == "" {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "consoleId and reference are required")

		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	path, err := storage.SaveRAMPath(root, owner, consoleID, reference)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid save path")

		return
	}

	answer := retroSaveRAMInfo{}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		modTime := info.ModTime()
		answer.Exists = true
		answer.Timestamp = &modTime
	}
	writeJSON(w, http.StatusOK, answer)
}

// retroSaveRAMInfo — Timestamp est absent (et non la date zéro de Go) quand il n'y a rien.
type retroSaveRAMInfo struct {
	Exists    bool       `json:"exists"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
}
