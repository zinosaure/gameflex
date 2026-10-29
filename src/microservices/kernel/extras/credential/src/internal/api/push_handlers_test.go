package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
)

// LE NAVIGATEUR S'ABONNE AVEC CE QU'IL ENVOIE VRAIMENT (2026-10-15).
//
// Chrome et Firefox envoient PushSubscription.toJSON() — {endpoint,
// expirationTime, keys}. Le décodeur du service refuse les champs inconnus,
// et `expirationTime` en était un : chaque abonnement prenait un 400, et
// aucune notification push n'est jamais partie. Le corps ci-dessous est celui
// que Chrome a envoyé à la release ce jour-là, aux clés près.
func TestAPushSubscriptionAsTheBrowserSendsItIsKept(t *testing.T) {
	config.DBPath = filepath.Join(t.TempDir(), "credential.db")
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := db.Exec(`INSERT INTO users (username, hashed_password, display_name, role_level) VALUES ('root', 'x', 'Root', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := result.LastInsertId()

	server := &Server{db: db}
	const endpoint = "https://fcm.googleapis.com/fcm/send/epreuve"
	body := `{"endpoint":"` + endpoint + `","expirationTime":null,"keys":{"p256dh":"BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM","auth":"tBHItJI5svbpez7KI4CCXg"}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/push/subscribe", strings.NewReader(body))
	server.handleSubscribe(recorder, request, auth.Principal{ID: userID, Username: "root"})

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/push/subscribe = %d %s", recorder.Code, recorder.Body.String())
	}
	var kept int
	if err := db.QueryRow(`SELECT COUNT(*) FROM push_subscriptions WHERE endpoint = ? AND user_id = ?`, endpoint, userID).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("%d abonnement(s) gardé(s), want 1", kept)
	}
}
