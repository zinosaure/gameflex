package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
)

// limiterAt — un limiteur dont l'horloge se déplace à la main.
func limiterAt(clock *time.Time) *loginLimiter {
	limiter := newLoginLimiter()
	limiter.now = func() time.Time { return *clock }

	return limiter
}

// CINQ ÉCHECS LIBRES, PUIS UNE MINUTE, QUI DOUBLE (#454) ; le bon mot de passe
// efface tout.
func TestFailedSignInsMakeTheNextOneWait(t *testing.T) {
	clock := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	limiter := limiterAt(&clock)
	for i := 0; i < loginFreeTries-1; i++ {
		limiter.fail("root", "203.0.113.7")
	}
	if wait := limiter.wait("root", "203.0.113.7"); wait != 0 {
		t.Fatalf("après %d échecs : attendre %v, attendu rien", loginFreeTries-1, wait)
	}
	limiter.fail("root", "203.0.113.7")
	if wait := limiter.wait("ROOT", "203.0.113.7"); wait != time.Minute {
		t.Fatalf("après %d échecs : attendre %v, attendu une minute (le nom sans casse)", loginFreeTries, wait)
	}
	if wait := limiter.wait("root", "198.51.100.9"); wait != 0 {
		t.Fatalf("une autre adresse : attendre %v, attendu rien", wait)
	}

	clock = clock.Add(time.Minute)
	limiter.fail("root", "203.0.113.7")
	if wait := limiter.wait("root", "203.0.113.7"); wait != 2*time.Minute {
		t.Fatalf("un échec de plus : attendre %v, attendu deux minutes", wait)
	}
	for i := 0; i < 10; i++ {
		limiter.fail("root", "203.0.113.7")
	}
	if wait := limiter.wait("root", "203.0.113.7"); wait != loginMaxWait {
		t.Fatalf("l'attente plafonne : %v, attendu %v", wait, loginMaxWait)
	}

	limiter.succeed("root", "203.0.113.7")
	if wait := limiter.wait("root", "203.0.113.7"); wait != 0 {
		t.Fatalf("après une connexion réussie : attendre %v", wait)
	}
}

// UNE ADRESSE INVENTÉE PAR ESSAI NE CONTOURNE PAS LA LIMITE : le compte, toutes
// adresses confondues, attend aussi après vingt échecs.
func TestInventedAddressesDoNotBypassTheLimit(t *testing.T) {
	clock := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	limiter := limiterAt(&clock)
	for i := 0; i < loginFreeTriesAccount; i++ {
		limiter.fail("root", "192.0.2."+string(rune('a'+i)))
	}
	if wait := limiter.wait("root", "192.0.2.250"); wait != time.Minute {
		t.Fatalf("une adresse neuve après %d échecs du compte : attendre %v, attendu une minute",
			loginFreeTriesAccount, wait)
	}
	if wait := limiter.wait("marc", "192.0.2.250"); wait != 0 {
		t.Fatalf("un autre compte : attendre %v", wait)
	}
}

// LA ROUTE : au-delà des essais libres, 429 et Retry-After, même avec le bon mot
// de passe — l'essai de trop ne dit pas s'il était juste.
func TestTheSignInRouteMakesAnAttackerWait(t *testing.T) {
	server, db := tokenServer(t)
	server.logins = newLoginLimiter()
	hashed, err := auth.HashPassword("le-bon-mot-de-passe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET hashed_password = ? WHERE username = 'admin'`, hashed); err != nil {
		t.Fatal(err)
	}
	login := func(password string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
		request.RemoteAddr = "203.0.113.7:51000"
		server.handleLogin(response, request)

		return response
	}

	for i := 0; i < loginFreeTries; i++ {
		if response := login("faux"); response.Code != http.StatusUnauthorized {
			t.Fatalf("échec %d : %d", i+1, response.Code)
		}
	}
	response := login("le-bon-mot-de-passe")
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("après %d échecs : %d (Retry-After %q), attendu 429", loginFreeTries, response.Code,
			response.Header().Get("Retry-After"))
	}

	server.logins.now = func() time.Time { return time.Now().Add(loginFirstWait + time.Second) }
	if response := login("le-bon-mot-de-passe"); response.Code != http.StatusOK {
		t.Fatalf("après l'attente, le bon mot de passe : %d %s", response.Code, response.Body.String())
	}
}
