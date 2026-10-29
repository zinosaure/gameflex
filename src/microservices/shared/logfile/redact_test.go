package logfile

import "testing"

// LE CAS RÉEL, celui repéré à l'écran le 2026-09-09 : le journal d'accès
// écrivait l'URL entière d'une poignée de main WebSocket, JWT compris.
func TestRedactHidesTheTokenInAnAccessLine(t *testing.T) {
	line := `2026/09/09 00:26:17 "GET http://192.168.1.50:9011/v4/ws?token=` +
		`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VybmFtZSI6Imdpbm8iLCJyb2xl` +
		`X2xldmVsIjoxLCJzdWIiOiIyIiwiZXhwIjoxNzg4OTc4MTE3fQ.qap00HGZ9G4QsHYkBeU3fYENkm` +
		`ddc0Wn3XIMULa3AvQ HTTP/1.1" from 172.18.0.4:55042 - 000 0B in 45.594658285s`

	got := Redact(line)
	if contains(got, "eyJhbGciOi") {
		t.Fatalf("le jeton est encore là : %s", got)
	}
	// La CLÉ reste : savoir qu'une requête portait un jeton est ce qui
	// permet de comprendre un refus d'authentification.
	if !contains(got, "token=<masqué>") {
		t.Errorf("le paramètre a disparu au lieu d'être masqué : %s", got)
	}
	// Le reste de la ligne doit survivre — c'est ce qu'on vient lire.
	for _, keep := range []string{"GET", "172.18.0.4:55042", "45.594658285s", "/v4/ws"} {
		if !contains(got, keep) {
			t.Errorf("%q a été emporté : %s", keep, got)
		}
	}
}

// Un secret dans un JSON de journal, et un paramètre qui suit.
func TestRedactHandlesJsonAndTrailingParams(t *testing.T) {
	got := Redact(`{"msg":"login","password":"hunter2","username":"gino"}`)
	if contains(got, "hunter2") {
		t.Errorf("mot de passe visible : %s", got)
	}
	if !contains(got, `"username":"gino"`) {
		t.Errorf("le champ suivant a été emporté : %s", got)
	}

	got = Redact(`/api/ws?token=abc123&sessionId=xyz`)
	if contains(got, "abc123") || !contains(got, "sessionId=xyz") {
		t.Errorf("découpage à l'esperluette raté : %s", got)
	}
}

// Ce qui ne porte pas de secret ne doit pas bouger : un journal masqué à
// l'excès ne se lit plus. « sessionId » ressemble à du base64 et n'est pas
// un secret — c'est même ce qu'on vient chercher.
func TestRedactLeavesOrdinaryLinesAlone(t *testing.T) {
	for _, line := range []string{
		`{"msg":"debug: frame pipeline stats","created":7680,"mapLen":60}`,
		`{"msg":"controller: button","sessionId":"id_60aac685aa380173","button":"SQUARE"}`,
	} {
		if Redact(line) != line {
			t.Errorf("ligne modifiée sans raison :\n  avant %s\n  après %s", line, Redact(line))
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}

		return false
	})()
}
