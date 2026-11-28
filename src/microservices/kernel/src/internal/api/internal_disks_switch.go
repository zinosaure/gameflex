package api

// COUPER UN DISQUE INTERNE, ET LE SUIVRE (chantier 86, `#813`) — docker/intdisks_state.go pour le pourquoi.
//
//	PUT /api/v1/retrogaming/disks/{name}/enabled   {enabled} — administration ; refusé (409, `disk_in_use`) tant
//	                                                qu'une partie monte un dossier du disque
//	sujet `disks` du centre de messages             la liste des disques, repoussée à chaque branchement,
//	                                                débranchement ou coupure — jamais relue à intervalle

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/docker"
	"gameflex/facade"
)

var (
	disksWatchMu sync.Mutex
	disksWatch   = map[chan struct{}]bool{}
)

// watchDisks — un réveil à chaque changement des disques ; `stop` le libère.
func watchDisks() (chan struct{}, func()) {
	wake := make(chan struct{}, 1)
	disksWatchMu.Lock()
	disksWatch[wake] = true
	disksWatchMu.Unlock()

	return wake, func() {
		disksWatchMu.Lock()
		delete(disksWatch, wake)
		disksWatchMu.Unlock()
	}
}

// disksChanged — réveille ceux qui suivent les disques.
func disksChanged() {
	disksWatchMu.Lock()
	defer disksWatchMu.Unlock()
	for wake := range disksWatch {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// followMounts — les montages de l'hôte (le dossier des disques, en `rslave`) : un disque branché ou débranché
// réveille la liste.
func (server *Server) followMounts() {
	docker.WatchMounts(context.Background(), disksChanged)
}

// diskViews — les disques tels que la page les montre (GET /disks, le sujet `disks`).
func (server *Server) diskViews(ctx context.Context) []internalDiskView {
	views := []internalDiskView{}
	for _, disk := range server.disks(ctx) {
		views = append(views, server.diskView(disk))
	}

	return views
}

// followDisks — le sujet `disks` : la liste tout de suite, puis à chaque changement.
func (server *Server) followDisks(ctx context.Context, conn *facade.Conn) {
	wake, stop := watchDisks()
	defer stop()

	facade.PushOnChange(ctx, conn, facade.Push{Type: "disks", Topic: "disks"}, wake, func() any {
		return server.diskViews(ctx)
	})
}

// handleInternalDiskEnabled — PUT /api/v1/retrogaming/disks/{name}/enabled.
func (server *Server) handleInternalDiskEnabled(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "expected {enabled}")

		return
	}
	var disk *docker.InternalDisk
	for _, candidate := range server.disks(r.Context()) {
		if candidate.Name == name {
			found := candidate
			disk = &found
		}
	}
	if disk == nil {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no internal disk named "+name)

		return
	}
	if !*body.Enabled && server.manager != nil {
		games, err := server.manager.SessionsUsingDisk(r.Context(), *disk)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, "storage_failed", err.Error())

			return
		}
		if len(games) > 0 {
			writeErrorCode(w, http.StatusConflict, "disk_in_use", "the internal disk "+name+" is used by a game in progress: "+strings.Join(games, ", "))

			return
		}
	}
	if err := docker.SetDiskEnabled(name, *body.Enabled); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "storage_failed", err.Error())

		return
	}
	disksChanged()

	for _, candidate := range server.disks(r.Context()) {
		if candidate.Name == name {
			writeJSON(w, http.StatusOK, server.diskView(candidate))

			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "enabled": *body.Enabled})
}
