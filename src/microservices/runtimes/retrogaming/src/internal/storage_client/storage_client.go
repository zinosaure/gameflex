// Package storage_client — where a session child finds the game data.
//
// The parent (the kernel) resolves each section's absolute path and hands it
// down in the environment (ROMS_PATH, CORES_PATH, BIOS_PATH, SAVES_PATH);
// AdoptEnv reads them once, at startup. A missing variable falls back to the
// root-relative default, so a child started by hand still works.
//
// It also knew how to ask the storage service over HTTP. Nothing ever gave it
// an address — every caller built it in "env-only" mode — and that service's
// only route is internal now (2026-10-02): the dead branch is gone with it.
package storage_client

import (
	"os"
	"path/filepath"
	"sync"
)

const (
	SectionRoms  = "roms"
	SectionCores = "cores"
	SectionBios  = "bios"
	SectionSaves = "saves"
)

var defaults = map[string]string{
	SectionRoms:  "roms",
	SectionCores: "cores",
	SectionBios:  "bios",
	SectionSaves: "saves",
}

var envKeys = map[string]string{
	SectionRoms:  "ROMS_PATH",
	SectionCores: "CORES_PATH",
	SectionBios:  "BIOS_PATH",
	SectionSaves: "SAVES_PATH",
}

type Client struct {
	rootPath string

	mu       sync.Mutex
	absolute map[string]string // what AdoptEnv found (path()'s source of truth)
}

func New(rootPath string) *Client {
	return &Client{
		rootPath: rootPath,
		absolute: make(map[string]string),
	}
}

// path resolves one section to its absolute directory.
func (client *Client) path(section string) string {
	client.mu.Lock()
	resolved := client.absolute[section]
	client.mu.Unlock()

	if resolved == "" {
		// Nothing adopted (e.g. a hand-run child): the plain root-relative
		// default.
		resolved = filepath.Join(client.rootPath, defaults[section])
	}

	return resolved
}

func (client *Client) Roms() string  { return client.path(SectionRoms) }
func (client *Client) Cores() string { return client.path(SectionCores) }
func (client *Client) Bios() string  { return client.path(SectionBios) }
func (client *Client) Saves() string { return client.path(SectionSaves) }

// AdoptEnv is called once at child startup. The values are already-resolved
// ABSOLUTE paths (the parent did the resolving), so they are used as-is.
// Missing variables keep the root-relative default.
func (client *Client) AdoptEnv() {
	client.mu.Lock()
	defer client.mu.Unlock()

	for section, envKey := range envKeys {
		if value := os.Getenv(envKey); value != "" {
			client.absolute[section] = value
		}
	}
}
