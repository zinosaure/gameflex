// Package presence replicates the Python presence registry in memory:
// one snapshot broadcast to every connection on each change, direct
// per-user delivery for invites. No heartbeat — clients reconnect on their
// own. State is deliberately NOT persisted: a restart empties the registry
// and each browser re-announces itself.
package presence

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

type Entry struct {
	UserID      int64   `json:"userId"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	Platform    *string `json:"platform"`
	ConsoleID   *string `json:"consoleId"`
	Reference   *string `json:"reference"`
	GoSessionID *string `json:"goSessionId"`
	Role        *string `json:"role"`
}

type connection struct {
	userID int64
	conn   *websocket.Conn
}

type Registry struct {
	mu          sync.Mutex
	entries     map[int64]*Entry // one entry per ONLINE user
	connections map[int64]map[*connection]struct{}
	notify      chan struct{} // one buffered signal per change burst
}

func NewRegistry() *Registry {
	registry := &Registry{
		entries:     map[int64]*Entry{},
		connections: map[int64]map[*connection]struct{}{},
		notify:      make(chan struct{}, 1),
	}
	go registry.broadcastLoop()
	return registry
}

// broadcastLoop drains change signals and re-broadcasts the full snapshot
// to every connection, exactly like the Python registry (no diffs).
func (registry *Registry) broadcastLoop() {
	for range registry.notify {
		registry.mu.Lock()
		users := make([]Entry, 0, len(registry.entries))
		for _, entry := range registry.entries {
			users = append(users, *entry)
		}
		message, err := json.Marshal(map[string]any{"type": "list", "users": users})
		if err == nil {
			for _, set := range registry.connections {
				for connection := range set {
					connection.conn.WriteMessage(websocket.TextMessage, message)
				}
			}
		}
		registry.mu.Unlock()
	}
}

func (registry *Registry) signal() {
	select {
	case registry.notify <- struct{}{}:
	default:
	}
}

// Connect registers a browser connection for a user and starts the message
// loop. Returns once the connection is closed.
func (registry *Registry) Connect(userID int64, username, displayName string, conn *websocket.Conn) {
	registry.mu.Lock()
	if registry.entries[userID] == nil {
		registry.entries[userID] = &Entry{UserID: userID, Username: username, DisplayName: displayName}
	}
	if registry.connections[userID] == nil {
		registry.connections[userID] = map[*connection]struct{}{}
	}
	connection := &connection{userID: userID, conn: conn}
	registry.connections[userID][connection] = struct{}{}
	registry.mu.Unlock()

	registry.signal()
	registry.readLoop(connection)
	registry.unregister(connection)
}

func (registry *Registry) unregister(connection *connection) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	set, ok := registry.connections[connection.userID]
	if !ok {
		return
	}
	delete(set, connection)
	if len(set) == 0 {
		delete(registry.connections, connection.userID)
		// Same behaviour as the Python registry: going offline when the
		// last connection closes removes the entry entirely.
		delete(registry.entries, connection.userID)
	}
	registry.signal()
}

func (registry *Registry) readLoop(connection *connection) {
	defer connection.conn.Close()

	for {
		var message struct {
			Type        string `json:"type"`
			Platform    string `json:"platform"`
			ConsoleID   string `json:"consoleId"`
			Reference   string `json:"reference"`
			GoSessionID string `json:"goSessionId"`
			Role        string `json:"role"`
		}
		if err := connection.conn.ReadJSON(&message); err != nil {
			return
		}

		registry.mu.Lock()
		entry := registry.entries[connection.userID]
		if entry == nil {
			registry.mu.Unlock()
			continue
		}
		switch message.Type {
		case "playing":
			role := message.Role
			if role == "" {
				role = "player"
			}
			setString(&entry.Platform, message.Platform)
			setString(&entry.ConsoleID, message.ConsoleID)
			setString(&entry.Reference, message.Reference)
			setString(&entry.GoSessionID, message.GoSessionID)
			entry.Role = &role
			registry.mu.Unlock()
			registry.signal()
		case "idle":
			entry.Platform, entry.ConsoleID = nil, nil
			entry.Reference, entry.GoSessionID, entry.Role = nil, nil, nil
			registry.mu.Unlock()
			registry.signal()
		default:
			// Malformed frames are ignored, connection kept (Python parity).
			registry.mu.Unlock()
		}
	}
}

func setString(target **string, value string) {
	if value == "" {
		*target = nil
	} else {
		copy := value
		*target = &copy
	}
}

// SendToUser delivers a payload to every connection of one user (invites).
// Returns whether at least one connection received it.
func (registry *Registry) SendToUser(userID int64, payload map[string]any) bool {
	registry.mu.Lock()
	set := registry.connections[userID]
	message, err := json.Marshal(payload)
	registry.mu.Unlock()
	if err != nil {
		return false
	}

	delivered := false
	for connection := range set {
		if connection.conn.WriteMessage(websocket.TextMessage, message) == nil {
			delivered = true
		}
	}
	return delivered
}

// Snapshot returns the current user list (used for diagnostics, not part of
// the WS protocol).
func (registry *Registry) Snapshot() []Entry {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	users := make([]Entry, 0, len(registry.entries))
	for _, entry := range registry.entries {
		users = append(users, *entry)
	}
	return users
}
