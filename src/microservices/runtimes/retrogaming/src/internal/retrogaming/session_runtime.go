package retrogaming

// Bits that outlived the parent process. `Handler` used to be the REST/WS
// surface of a long-lived service (handlers.go, ~500 lines, deleted with the
// supervisor); what remains of it is the four path accessors the session
// itself needs, and the shape `serveEmulationSession` still hangs off.
// readProcTimes came from the supervisor, which sampled its children — this
// process now samples itself for its own /status.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Handler holds no paths of its own — they belong to the storage service
// (internal/storage_client), which resolved them before this container was
// started and passed them down as env vars.
type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) romsRoot() string  { return storagePaths.Roms() }
func (h *Handler) coresRoot() string { return storagePaths.Cores() }
func (h *Handler) savesRoot() string { return storagePaths.Saves() }
func (h *Handler) biosRoot() string  { return storagePaths.Bios() }

// readProcTimes reads a process's utime/stime (clock ticks) from
// /proc/<pid>/stat and its resident memory from /proc/<pid>/status. The
// comm field may contain spaces and ')', so the stat line is parsed from
// the LAST ')' onward (fields 3..; utime=14 → index 11, stime=15 → 12).
func readProcTimes(pid int) (utime, stime, rss int64) {
	statBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return
	}
	lastParen := strings.LastIndexByte(string(statBytes), ')')
	if lastParen < 0 {
		return
	}
	fields := strings.Fields(string(statBytes[lastParen+1:]))
	if len(fields) > 12 {
		utime, _ = strconv.ParseInt(fields[11], 10, 64)
		stime, _ = strconv.ParseInt(fields[12], 10, 64)
	}

	statusBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(statusBytes), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			kb := strings.TrimSpace(strings.TrimPrefix(line, "VmRSS:"))
			kb = strings.TrimSuffix(kb, " kB")
			if v, err := strconv.ParseInt(kb, 10, 64); err == nil {
				rss = v * 1024
			}
			break
		}
	}
	return
}
