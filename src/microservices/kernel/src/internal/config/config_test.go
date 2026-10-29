package config

import "testing"

func TestTheMirrorIsAnHTTPAddressOrNothing(t *testing.T) {
	for raw, want := range map[string]string{
		"":                                    "",
		"https://mirror.example.org/gameflex": "https://mirror.example.org/gameflex",
		"http://192.0.2.4:8080/":              "http://192.0.2.4:8080/",
	} {
		got, err := mirrorURL(raw)
		if err != nil || got != want {
			t.Errorf("mirrorURL(%q) = %q, %v ; attendu %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "/gameflex/cores", "ftp://mirror.example.org", "https://"} {
		if got, err := mirrorURL(raw); err == nil {
			t.Errorf("mirrorURL(%q) = %q : refus attendu", raw, got)
		}
	}
}

// Le miroir part de la variable du compose, sans les blancs autour.
func TestTheMirrorComesFromItsVariable(t *testing.T) {
	t.Setenv("GAMEFLEX_PUBLIC_HOST", "192.0.2.10")
	t.Setenv("GAMEFLEX_MIRROR_URL", " https://mirror.example.org/gameflex ")
	loaded, err := Load()
	if err != nil || loaded.MirrorURL != "https://mirror.example.org/gameflex" {
		t.Fatalf("Load() = %q, %v", loaded.MirrorURL, err)
	}

	t.Setenv("GAMEFLEX_MIRROR_URL", "mirror.example.org")
	if _, err := Load(); err == nil {
		t.Fatal("une adresse sans http(s) doit arrêter le démarrage")
	}
}

// L'ADRESSE DE CETTE MACHINE N'A PAS DE DÉFAUT (C1) : c'était celle de la
// machine de développement, et une installation qui l'oubliait ne le savait pas.
func TestThePublicHostIsRequired(t *testing.T) {
	t.Setenv("GAMEFLEX_PUBLIC_HOST", "")
	if _, err := Load(); err == nil {
		t.Fatal("sans GAMEFLEX_PUBLIC_HOST, le démarrage doit s'arrêter")
	}
	t.Setenv("GAMEFLEX_PUBLIC_HOST", "192.0.2.10")
	if loaded, err := Load(); err != nil || loaded.PublicHost != "192.0.2.10" {
		t.Fatalf("Load() = %q, %v", loaded.PublicHost, err)
	}
}
