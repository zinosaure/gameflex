package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/web_push"
)

func (server *Server) handleVapidPublicKey(w http.ResponseWriter, r *http.Request) {
	respondItem(w, http.StatusOK, map[string]any{"publicKey": server.web_push.PublicKeyBase64URL()})
}

func (server *Server) handleSubscribe(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	// LE CORPS TEL QUE LE NAVIGATEUR L'ENVOIE : PushSubscription.toJSON(),
	// soit {endpoint, expirationTime, keys}. decodeBody refuse les champs
	// inconnus, et `expirationTime` en était un — chaque abonnement prenait un
	// 400, et aucune notification push n'est jamais partie (trouvé le
	// 2026-10-15 en les remettant). Lu, pas gardé : aucun service de poussée
	// courant n'en donne.
	var body struct {
		Endpoint       string `json:"endpoint"`
		ExpirationTime *int64 `json:"expirationTime"`
		Keys           struct {
			P256DH string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	// Upsert by endpoint — re-subscribing re-attaches the row to the
	// current user (a fresh browser install gets a new endpoint anyway).
	var existingID int64
	err := server.db.QueryRow(`SELECT id FROM push_subscriptions WHERE endpoint = ?`, body.Endpoint).Scan(&existingID)
	if err == nil {
		_, _ = server.db.Exec(
			`UPDATE push_subscriptions SET user_id = ?, p256dh = ?, auth = ? WHERE id = ?`,
			principal.ID, body.Keys.P256DH, body.Keys.Auth, existingID,
		)
	} else {
		_, err = server.db.Exec(
			`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, created_at) VALUES (?, ?, ?, ?, ?)`,
			principal.ID, body.Endpoint, body.Keys.P256DH, body.Keys.Auth, now(),
		)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}
	respondItem(w, http.StatusOK, map[string]any{"success": true})
}

func (server *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		respondError(w, http.StatusBadRequest, "Missing endpoint.")
		return
	}
	// Only the owning user's rows are removed — no-op otherwise.
	_, _ = server.db.Exec(`DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`, principal.ID, endpoint)
	respondItem(w, http.StatusOK, map[string]any{"success": true})
}

func (server *Server) handleInvite(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		ToUsername string `json:"to_username"`
		Platform   string `json:"platform"`
		ConsoleID  string `json:"console_id"`
		Reference  string `json:"reference"`
		SessionID  string `json:"session_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	var recipientID int64
	var recipientName string
	err := server.db.QueryRow(`SELECT id, display_name FROM users WHERE username = ?`, body.ToUsername).Scan(&recipientID, &recipientName)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "No account with that username.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	payload := map[string]any{
		"type":      "invite",
		"from":      map[string]any{"userId": principal.ID, "displayName": principal.DisplayName},
		"platform":  body.Platform,
		"consoleId": body.ConsoleID,
		"reference": body.Reference,
		"sessionId": body.SessionID,
	}

	deliveredLive := server.presence.SendToUser(recipientID, payload)

	// Web Push fallback for closed tabs — sent from a goroutine so a slow
	// push service never blocks the invite response.
	go server.sendPushToUser(recipientID, payload)

	respondItem(w, http.StatusOK, map[string]any{"success": true, "deliveredLive": deliveredLive})
}

func (server *Server) sendPushToUser(userID int64, payload map[string]any) {
	rows, err := server.db.Query(`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ?`, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	body, err := marshalPayload(payload)
	if err != nil {
		return
	}

	for rows.Next() {
		var subscription web_push.Subscription
		if err := rows.Scan(&subscription.Endpoint, &subscription.P256DH, &subscription.Auth); err != nil {
			continue
		}
		err := server.web_push.Send(subscription, body)
		switch {
		case errors.Is(err, web_push.ErrSubscriptionGone):
			// The push service no longer knows this subscription.
			_, _ = server.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, subscription.Endpoint)
		case err != nil:
			slog.Warn("push delivery failed", "endpoint", subscription.Endpoint, "error", err)
		}
	}
}
