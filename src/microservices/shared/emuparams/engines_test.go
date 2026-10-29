package emuparams

import (
	"errors"
	"os"
	"testing"
)

var shadps4 = Ref{ConsoleID: "ps4", Emulator: "shadps4"}

// valueAt — la valeur qu'un lancement écrirait à cette adresse, "" s'il n'y
// écrit rien.
func valueAt(settings Settings, config string) string {
	for _, entry := range settings.Entries {
		if entry.Config == config {
			return entry.Value
		}
	}

	return ""
}

func engineOf(item Emulator, engine string) EngineSetting {
	for _, setting := range item.Engines {
		if setting.Engine == engine {
			return setting
		}
	}

	return EngineSetting{}
}

const dynarec = "Environment > BOX64_DYNAREC"

// UN MOTEUR DÉSIGNE UN PRÉRÉGLAGE (chantier 36) : enregistré avec l'émulateur,
// ses valeurs passent par-dessus celles du fichier pour un jeu de ce moteur —
// le moteur gagne —, et pour lui seul ; coupé, il ne pose plus rien.
func TestAnEngineLaysItsPresetOverTheEmulatorFile(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Save(shadps4, "PlayStation 4", nil, map[string]any{"dynarec": true}, "",
		[]EngineSetting{{Engine: "unity", Preset: "interpreter", Enabled: true}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(saved.Engines) != len(engineNames) {
		t.Fatalf("%d moteurs à l'écran, attendu %d", len(saved.Engines), len(engineNames))
	}
	if unity := engineOf(saved, "unity"); unity.Preset != "interpreter" || !unity.Enabled {
		t.Fatalf("unity = %+v, attendu interpreter et actif", unity)
	}

	if got := valueAt(store.SettingsFor(shadps4, ""), dynarec); got != "true" {
		t.Fatalf("sans moteur : %s = %q, attendu la valeur du fichier, true", dynarec, got)
	}
	unity := store.SettingsFor(shadps4, "unity")
	if got := valueAt(unity, dynarec); got != "false" {
		t.Fatalf("Unity : %s = %q, attendu celle du préréglage interpreter, false", dynarec, got)
	}
	if unity.Engine != "unity" || unity.EnginePreset != "interpreter" {
		t.Fatalf("Unity : %+v, le moteur et son préréglage ne sont pas dits", unity)
	}
	if got := valueAt(store.SettingsFor(shadps4, "unreal"), dynarec); got != "true" {
		t.Fatalf("Unreal, sans préréglage : %s = %q, attendu true", dynarec, got)
	}

	if _, err := store.Save(shadps4, "PlayStation 4", nil, nil, "",
		[]EngineSetting{{Engine: "unity", Preset: "interpreter", Enabled: false}}); err != nil {
		t.Fatalf("Save (coupé) : %v", err)
	}
	if got := valueAt(store.SettingsFor(shadps4, "unity"), dynarec); got != "true" {
		t.Fatalf("Unity coupé : %s = %q, attendu true", dynarec, got)
	}
}

// UN PRÉRÉGLAGE INCONNU EST REFUSÉ, ET RIEN N'EST ÉCRIT : le moteur poserait des
// valeurs que personne ne voit. De même pour une console dont les jeux n'ont
// pas de moteur reconnu (la PS3). Mais le préréglage que ce même enregistrement
// crée se désigne tout de suite.
func TestAnEngineSettingIsCheckedBeforeAnythingIsWritten(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	var refused *RefusedError
	_, err := store.Save(shadps4, "", nil, map[string]any{"dynarec": false}, "",
		[]EngineSetting{{Engine: "unity", Preset: "nope", Enabled: true}})
	if !errors.As(err, &refused) {
		t.Fatalf("préréglage inconnu : %v, attendu un refus", err)
	}
	if _, err := os.Stat(store.emulatorFile(shadps4)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refusé, le fichier de l'émulateur a quand même été écrit (%v)", err)
	}
	if _, err := store.Save(rpcs3, "", nil, nil, "", []EngineSetting{{Engine: "unity", Enabled: true}}); !errors.As(err, &refused) {
		t.Fatalf("un moteur pour la PS3 : %v, attendu un refus", err)
	}
	if _, err := store.Save(shadps4, "", nil, map[string]any{"dynarec": false}, "Unity S25",
		[]EngineSetting{{Engine: "unity", Preset: "unity-s25", Enabled: true}}); err != nil {
		t.Fatalf("le préréglage créé par le même enregistrement : %v", err)
	}
	if got := valueAt(store.SettingsFor(shadps4, "unity"), dynarec); got != "false" {
		t.Fatalf("Unity S25 : %s = %q, attendu false", dynarec, got)
	}
}

// SANS FICHIER D'ÉMULATEUR (ou coupé), CE QUE LE MOTEUR POSE S'ÉCRIT QUAND MÊME :
// il n'en dépend pas. Et un préréglage disparu se dit au lieu de se taire.
func TestAnEngineAppliesWithoutTheEmulatorFile(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Root+"/params/ps4/engines", 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		if err := os.WriteFile(store.engineFilePath("ps4", "unity"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"system": "shadPS4", "preset": "interpreter"}`)
	settings := store.SettingsFor(shadps4, "unity")
	if settings.State != "absent" || valueAt(settings, dynarec) != "false" {
		t.Fatalf("sans fichier : %+v, attendu absent et %s = false", settings, dynarec)
	}

	write(`{"system": "shadPS4", "preset": "gone"}`)
	if settings := store.SettingsFor(shadps4, "unity"); settings.EngineError == "" || len(settings.Entries) != 0 {
		t.Fatalf("préréglage disparu : %+v, attendu l'erreur dite et rien d'écrit", settings)
	}
}
