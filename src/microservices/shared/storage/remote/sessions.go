package remote

// LES SESSIONS CHEZ NOS AMIS (chantier 20) — le même geste dans les deux
// kernels : échanger le jeton qu'un membre d'ici a reçu chez un ami (le jeton
// de SON compte là-bas, collé une fois à la place de l'identifiant et du mot de
// passe d'autrefois) contre une session à son nom, la garder tant qu'elle
// vaut, et la vérifier chez l'ami avant de la rendre. Le jeton ne sort pas du
// kernel ; la session, si : la page en a besoin pour parler à l'hôte (son
// canal, le flux d'une partie).
//
// PAR AMI ET PAR MEMBRE : deux membres d'ici qui ont chacun un compte chez le
// même ami y jouent chacun sous le sien. Le jeton du LIEN, lui, n'ouvre jamais
// de session — le catalogue seulement.

import (
	"sync"
	"time"
)

// SessionMargin — une session qui finit avant ce délai se ré-échange : une
// partie lancée avec elle ne doit pas tomber au bout de quelques minutes.
const SessionMargin = time.Hour

// Sessions — les sessions ouvertes chez nos amis, par ami et par membre d'ici.
// Zéro-valeur utilisable.
//
// Un seul verrou pour tous : deux pages qui demandent en même temps n'ouvrent
// qu'une session (chaque échange compte « une connexion » chez l'ami), et un
// ami qui ne répond pas retient au plus le délai d'une liste.
type Sessions struct {
	mu   sync.Mutex
	held map[sessionKey]heldSession
}

// sessionKey — un ami (son identifiant au registre) et un membre d'ici (son
// nom d'utilisateur).
type sessionKey struct {
	server int64
	member string
}

// heldSession — une session, et le jeton qui l'a ouverte : un jeton recollé
// n'hérite pas de la session de l'ancien.
type heldSession struct {
	token   string
	session Session
}

// At — une session chez cet ami pour ce membre, avec le jeton de son compte
// là-bas : celle qu'on garde, si elle vaut encore et que l'ami l'accepte
// toujours ; sinon une nouvelle. ErrDenied si l'ami refuse ce jeton
// (désactivé, échu, régénéré, compte supprimé) ; une autre erreur s'il ne
// répond pas.
func (sessions *Sessions) At(server int64, link, member, token string) (Session, error) {
	client, err := New(link, token)
	if err != nil {
		return Session{}, err
	}
	key := sessionKey{server: server, member: member}

	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	if held, found := sessions.held[key]; found && held.token == token &&
		time.Until(held.session.ExpiresAt) > SessionMargin {
		now, alive, err := client.SessionNow(held.session)
		if err != nil {
			return Session{}, err
		}
		if alive {
			sessions.hold(key, token, now)

			return now, nil
		}
	}
	delete(sessions.held, key)

	session, err := client.OpenSession()
	if err != nil {
		return Session{}, err
	}
	sessions.hold(key, token, session)

	return session, nil
}

// Hold — garder la session qu'un jeton vient d'ouvrir : celui qu'un membre
// colle est vérifié par un échange, et cet échange compte déjà une connexion
// chez l'ami ; la première partie n'a pas à en ouvrir une seconde.
func (sessions *Sessions) Hold(server int64, member, token string, session Session) {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	sessions.hold(sessionKey{server: server, member: member}, token, session)
}

// Forget — oublier la session d'un membre chez cet ami : il a retiré son jeton.
func (sessions *Sessions) Forget(server int64, member string) {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	delete(sessions.held, sessionKey{server: server, member: member})
}

func (sessions *Sessions) hold(key sessionKey, token string, session Session) {
	if sessions.held == nil {
		sessions.held = map[sessionKey]heldSession{}
	}
	sessions.held[key] = heldSession{token: token, session: session}
}
