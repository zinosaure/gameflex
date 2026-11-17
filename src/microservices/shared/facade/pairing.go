package facade

// LE SUJET `pairing` (chantier 60) — les modes découverte et les codes
// d'appairage, tels que le service des comptes les pousse : par la trame
// `pairing` de /internal/accounts/ws au serveur, par OnPairingChanged au
// téléphone. Les deux kernels y branchent la même chose : la dernière trame,
// et la part qu'une connexion a le droit d'en voir.
//
//   - chacun y voit son mode compte, et ses propres codes ;
//   - un administrateur y voit en plus le mode serveur, et les codes `federation`.
//
// Une page qui se reconnecte repart de l'instantané : `ended` lui dit ce
// qu'est devenu son code, `closed` pourquoi son mode s'est fermé. La valeur
// d'un code n'y passe jamais : seule la réponse de sa création la porte.

import (
	"context"
	"encoding/json"
	"sync"
)

// PairingFrame — la trame du service des comptes.
type PairingFrame struct {
	Server   *PairingServerMode   `json:"server"`
	Accounts []PairingAccountMode `json:"accounts"`
	Codes    []PairingCode        `json:"codes"`
	Ended    []PairingEnded       `json:"ended"`
	Closed   []PairingClosed      `json:"closed"`
}

// PairingServerMode — le mode serveur ouvert : jusqu'à quand, par qui.
type PairingServerMode struct {
	OpenUntil string `json:"open_until"`
	OpenedBy  string `json:"opened_by"`
}

// PairingAccountMode — le mode compte ouvert d'un membre.
type PairingAccountMode struct {
	Username  string `json:"username,omitempty"`
	OpenUntil string `json:"open_until"`
}

// PairingCode — un code vivant : sa sorte, son auteur, son échéance.
type PairingCode struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Author    string `json:"author"`
	ExpiresAt string `json:"expires_at"`
}

// PairingEnded — un code fini depuis moins d'une minute : `used` (par `by`,
// depuis l'adresse `from`), `expired`, `cancelled`, `too_many_failures`.
type PairingEnded struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Author string `json:"author"`
	Reason string `json:"reason"`
	By     string `json:"by,omitempty"`
	From   string `json:"from,omitempty"`
	At     string `json:"at"`
}

// PairingClosed — un mode fermé depuis moins d'une minute, et sa cause :
// `timeout`, `owner`, `too_many_failures`.
type PairingClosed struct {
	Scope    string `json:"scope"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
	At       string `json:"at"`
}

// PairingView — ce qu'une connexion voit de la trame.
type PairingView struct {
	// Server — le mode serveur, à un administrateur seulement.
	Server  *PairingServerMode  `json:"server"`
	Account *PairingAccountMode `json:"account"`
	Codes   []PairingCode       `json:"codes"`
	Ended   []PairingEnded      `json:"ended"`
	Closed  []PairingClosed     `json:"closed"`
}

// View — la part de `username` : son mode compte et ses codes ; pour un
// administrateur, le mode serveur et les codes `federation` en plus.
func (frame PairingFrame) View(username string, admin bool) PairingView {
	view := PairingView{Codes: []PairingCode{}, Ended: []PairingEnded{}, Closed: []PairingClosed{}}
	if admin {
		view.Server = frame.Server
	}
	for _, mode := range frame.Accounts {
		if mode.Username == username {
			view.Account = &PairingAccountMode{OpenUntil: mode.OpenUntil}
		}
	}
	sees := func(kind, author string) bool {
		return author == username || (admin && kind == "federation")
	}
	for _, code := range frame.Codes {
		if sees(code.Kind, code.Author) {
			view.Codes = append(view.Codes, code)
		}
	}
	for _, ended := range frame.Ended {
		if sees(ended.Kind, ended.Author) {
			view.Ended = append(view.Ended, ended)
		}
	}
	for _, closed := range frame.Closed {
		if (closed.Scope == "account" && closed.Username == username) || (closed.Scope == "server" && admin) {
			view.Closed = append(view.Closed, closed)
		}
	}

	return view
}

// Pairing — la dernière trame d'un kernel, et qui l'attend. La valeur zéro
// est prête : aucun mode ouvert.
type Pairing struct {
	mu       sync.Mutex
	frame    PairingFrame
	watchers map[chan struct{}]struct{}
}

// Set — une trame arrive : elle remplace la précédente, et chaque abonné se
// réveille.
func (pairing *Pairing) Set(raw []byte) error {
	var frame PairingFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return err
	}
	pairing.mu.Lock()
	defer pairing.mu.Unlock()

	pairing.frame = frame
	for watcher := range pairing.watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}

	return nil
}

// Frame — la dernière trame.
func (pairing *Pairing) Frame() PairingFrame {
	pairing.mu.Lock()
	defer pairing.mu.Unlock()

	return pairing.frame
}

// Watch — un réveil à chaque trame, et de quoi s'en défaire.
func (pairing *Pairing) Watch() (<-chan struct{}, func()) {
	pairing.mu.Lock()
	defer pairing.mu.Unlock()

	if pairing.watchers == nil {
		pairing.watchers = map[chan struct{}]struct{}{}
	}
	watcher := make(chan struct{}, 1)
	pairing.watchers[watcher] = struct{}{}

	return watcher, func() {
		pairing.mu.Lock()
		defer pairing.mu.Unlock()
		delete(pairing.watchers, watcher)
	}
}

// Follow — le sujet `pairing` d'une connexion : sa part tout de suite, puis à
// chaque trame qui la change.
func (pairing *Pairing) Follow(ctx context.Context, conn *Conn, admin bool) {
	wake, stop := pairing.Watch()
	defer stop()

	PushOnChange(ctx, conn, Push{Type: "pairing", Topic: "pairing"}, wake, func() any {
		return pairing.Frame().View(conn.Username(), admin)
	})
}
