package api

import (
	"context"
	"net/http"

	"gameflex/federation/internal/store"
)

// contextWith — la requête porte le jeton qui l'a ouverte, pour les handlers.
func contextWith(r *http.Request, token store.Token) context.Context {
	return context.WithValue(r.Context(), tokenKey{}, token)
}

// tokenOf — le jeton de la requête (vide hors de la surface fédérée).
func tokenOf(r *http.Request) store.Token {
	token, _ := r.Context().Value(tokenKey{}).(store.Token)

	return token
}
