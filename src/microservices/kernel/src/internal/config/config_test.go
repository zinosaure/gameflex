package config

import "testing"

// LES RÉGLAGES DE LA PAGE (chantier 81) : nettoyés, ou refusés avec la raison.
func TestTheLiveSettingsAreCheckedBeforeTheyApply(t *testing.T) {
	got, err := Live{ServerName: "  GameFlex  ", Players: map[string]int{"switch": 8, "wiiu": 2}, ConsoleLimits: map[string]int{"switch": 2, "ps3": 3, "ps5": 0}}.Normalized()
	if err != nil || got.ServerName != "GameFlex" || len(got.ConsoleLimits) != 2 || got.Players["wiiu"] != 2 {
		t.Fatalf("Normalized() = %+v, %v", got, err)
	}
	if got.Limits()["ps3"] != 3 {
		t.Fatalf("Limits() = %v", got.Limits())
	}
	for _, wrong := range []Live{
		{Players: map[string]int{"switch": 0}},
		{Players: map[string]int{"switch": 9}},
		{ConsoleLimits: map[string]int{"switch": 9}},
		{ConsoleLimits: map[string]int{"Switch 2": 1}},
		{GPU: "c4:00.0"},
		{ServerName: "0123456789012345678901234567890123456789x"},
	} {
		if _, err := wrong.Normalized(); err == nil {
			t.Errorf("Normalized(%+v) : refus attendu", wrong)
		}
	}
	if _, err := DefaultLive().Normalized(); err != nil {
		t.Fatalf("les défauts doivent passer : %v", err)
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
