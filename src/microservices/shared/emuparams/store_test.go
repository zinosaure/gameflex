package emuparams

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var rpcs3 = Ref{ConsoleID: "ps3", Emulator: "rpcs3"}

func readRpcs3(t *testing.T, store Store) file {
	t.Helper()
	content, err := os.ReadFile(store.emulatorFile(rpcs3))
	if err != nil {
		t.Fatalf("rpcs3.json : %v", err)
	}
	f, err := decode(content)
	if err != nil {
		t.Fatalf("rpcs3.json illisible : %v\n%s", err, content)
	}

	return f
}

// CE QUE GAMEFLEX LIVRE EST JUSTE : le fichier de chaque émulateur se lit, et
// chaque préréglage livré ne remplit que des paramètres qui existent, avec des
// valeurs qu'ils acceptent — une valeur sautée en silence ferait enregistrer
// autre chose que ce que le préréglage promet. Le préréglage par défaut est la
// liste livrée elle-même.
func TestTheShippedParamsAreValid(t *testing.T) {
	if len(Shipped()) == 0 {
		t.Fatal("aucun émulateur livré")
	}
	for _, ref := range Shipped() {
		definition, _ := shippedDefinition(ref)
		if _, ok := writers[ref.Emulator]; !ok {
			t.Fatalf("%s est livré sans écrivain : aucun lancement ne l'appliquerait", ref.Emulator)
		}
		names, contents := shippedPresets(ref.ConsoleID)
		if len(names) == 0 {
			t.Fatalf("%s : aucun préréglage", ref.Emulator)
		}
		for _, name := range names {
			var p preset
			if err := json.Unmarshal(contents[name], &p); err != nil {
				t.Fatalf("%s/%s : %v", ref.ConsoleID, name, err)
			}
			if !strings.EqualFold(p.System, definition.System) {
				continue
			}
			if p.Override == nil || *p.Override {
				t.Fatalf("%s/%s : un préréglage livré se dit « \"override\": false »", ref.ConsoleID, name)
			}
			for id, value := range p.Values {
				target, known := definition.Params.get(id)
				if !known {
					t.Fatalf("%s/%s : %s n'existe pas", ref.ConsoleID, name, id)
				}
				if _, err := target.accept(value); err != nil {
					t.Fatalf("%s/%s : %s : %v", ref.ConsoleID, name, id, err)
				}
			}
			if name == "default" {
				for _, id := range definition.Params.keys {
					want, _ := scalarText(definition.Params.values[id].Value)
					got, _ := scalarText(p.Values[id])
					if got != want {
						t.Fatalf("default.%s = %q, la liste livrée dit %q", id, got, want)
					}
				}
			}
		}
	}
}

// CE QUE L'APPLICATION DU TÉLÉPHONE IMPOSAIT À RPCS3 avant ce fichier
// (GlibcGuest.stagePs3Speed, jusqu'au 2026-10-12) est sa valeur livrée : le
// premier lancement après la mise à jour ne doit rien perdre. Les noms
// d'options sont ceux de RPCS3 — un nom périmé est refusé en silence.
func TestTheShippedRpcs3KeepsWhatThePhoneImposed(t *testing.T) {
	f, _ := shippedDefinition(rpcs3)
	want := map[string]any{
		"Video > Resolution Scale":        "auto",
		"Core > SPU Block Size":           "Mega",
		"Core > Max LLVM Compile Threads": "8",
		"Video > Multithreaded RSX":       true,
		"Video > Shader Mode":             "Async Recompiler with Shader Interpreter",
		"Audio > Enable Time Stretching":  true,
	}
	for _, id := range f.Params.keys {
		p := f.Params.values[id]
		if expected, ok := want[normalConfig(p.Config)]; ok {
			if p.Value != expected {
				t.Errorf("%s = %v, attendu %v", p.Config, p.Value, expected)
			}
			delete(want, normalConfig(p.Config))
		}
	}
	for config := range want {
		t.Errorf("%s manque au fichier livré", config)
	}
	if !f.enabled() || f.System != "RPCS3" {
		t.Fatalf("enabled %v, système %q", f.enabled(), f.System)
	}
}

// LE TÉLÉPHONE pose le fichier au premier démarrage ; supprimé ensuite, il ne
// revient pas. LE SERVEUR ne le pose jamais. Les préréglages livrés sont remis
// d'aplomb à chaque démarrage, ceux de l'admin jamais touchés, et chaque
// écriture passe par AfterWrite.
func TestEnsureSeedsOnlyWhereAsked(t *testing.T) {
	var touched []string
	phone := Store{Root: t.TempDir(), SeedOnFirstRun: true, AfterWrite: func(path string) { touched = append(touched, path) }}
	if err := phone.Ensure(); err != nil {
		t.Fatal(err)
	}
	f := readRpcs3(t, phone)
	if !f.enabled() || f.System != "RPCS3" {
		t.Fatalf("rpcs3.json posé : %+v", f)
	}
	if !strings.Contains(strings.Join(touched, "\n"), filepath.Join(phone.Root, "params")+"\n") ||
		!strings.Contains(strings.Join(touched, "\n"), phone.emulatorFile(rpcs3)) {
		t.Fatalf("AfterWrite n'a pas vu les dossiers et le fichier : %v", touched)
	}

	if err := os.Remove(phone.emulatorFile(rpcs3)); err != nil {
		t.Fatal(err)
	}
	shippedOne := filepath.Join(phone.presetsDir("ps3"), "hd.json")
	if err := os.WriteFile(shippedOne, []byte(`{"abîmé": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(phone.presetsDir("ps3"), "tekken.json")
	if err := os.WriteFile(mine, []byte(`à moi`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := phone.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(phone.emulatorFile(rpcs3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rpcs3.json supprimé est revenu")
	}
	if restored, _ := os.ReadFile(shippedOne); strings.Contains(string(restored), "abîmé") {
		t.Fatal("le préréglage livré n'a pas été remis d'aplomb")
	}
	if kept, _ := os.ReadFile(mine); string(kept) != "à moi" {
		t.Fatal("le préréglage de l'admin a été touché")
	}

	server := Store{Root: t.TempDir()}
	if err := server.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(server.emulatorFile(rpcs3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("le serveur ne doit pas poser rpcs3.json")
	}
	if _, err := os.Stat(filepath.Join(server.presetsDir("ps3"), "default.json")); err != nil {
		t.Fatalf("les préréglages du serveur : %v", err)
	}
	if settings := server.Settings(rpcs3); settings.State != "absent" || len(settings.Entries) != 0 {
		t.Fatalf("sans fichier : %+v", settings)
	}
}

// LE FORMULAIRE : l'interrupteur et les valeurs, écrits sans rien déranger
// d'autre — l'ordre des paramètres reste celui du fichier, « > » reste
// lisible. Sans fichier, l'enregistrer le crée ; illisible, le remplace.
func TestSaveWritesValuesAndEnabled(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}

	item := store.Describe(rpcs3, "PlayStation 3")
	if item.Exists || item.Enabled || len(item.Params) == 0 || item.Params[0].Options[0].Value != "auto" || item.Params[0].Options[1].Value != "50" {
		t.Fatalf("sans fichier : %+v", item)
	}

	off := false
	saved, err := store.Save(rpcs3, "PlayStation 3", &off, map[string]any{"spuBlockSize": "Giga", "performanceOverlay": true, "maxSpursThreads": 4.0}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Exists || saved.Enabled {
		t.Fatalf("après enregistrement : exists %v enabled %v", saved.Exists, saved.Enabled)
	}
	f := readRpcs3(t, store)
	shippedFile, _ := shippedDefinition(rpcs3)
	if f.Params.values["spuBlockSize"].Value != "Giga" || f.Params.values["performanceOverlay"].Value != true ||
		f.Params.values["maxSpursThreads"].Value != "4" || strings.Join(f.Params.keys, ",") != strings.Join(shippedFile.Params.keys, ",") {
		t.Fatal("valeurs ou ordre du fichier")
	}
	content, _ := os.ReadFile(store.emulatorFile(rpcs3))
	if !strings.HasPrefix(string(content), "{\n  \"enabled\": false,") || !strings.Contains(string(content), `"Video > Performance Overlay > Enabled"`) {
		t.Fatalf("fichier écrit :\n%s", content[:120])
	}

	if err := os.WriteFile(store.emulatorFile(rpcs3), []byte(`{"enabled": tru`), 0o644); err != nil {
		t.Fatal(err)
	}
	if item := store.Describe(rpcs3, ""); item.Error == "" || item.Enabled || len(item.Params) == 0 {
		t.Fatalf("illisible : %+v", item)
	}
	if _, err := store.Save(rpcs3, "", nil, map[string]any{"frameLimit": "30"}, "", nil); err != nil {
		t.Fatalf("réparer : %v", err)
	}
	if f := readRpcs3(t, store); !f.enabled() || f.Params.values["frameLimit"].Value != "30" {
		t.Fatal("le fichier n'a pas été réparé")
	}
}

// UNE VALEUR HORS DE SON TYPE est refusée, et rien n'est écrit — pas même les
// valeurs justes envoyées avec elle.
func TestSaveRefusesValuesOutsideTheirType(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.emulatorFile(rpcs3))
	for _, values := range []map[string]any{
		{"spuBlockSize": "Tera"},
		{"multithreadedRsx": "peut-être"},
		{"inconnu": "1"},
		{"msaa": "Disabled", "frameLimit": "45"},
	} {
		_, err := store.Save(rpcs3, "", nil, values, "", nil)
		var refused *RefusedError
		if !errors.As(err, &refused) {
			t.Errorf("%v : %v, attendu un refus", values, err)
		}
	}
	if after, _ := os.ReadFile(store.emulatorFile(rpcs3)); string(after) != string(before) {
		t.Fatal("un refus a quand même écrit")
	}
}

// LES PRÉRÉGLAGES remplissent le formulaire : l'écran reçoit leurs valeurs déjà
// passées par les paramètres, et ce qui ne remplit rien est dit. Un préréglage
// pour un autre émulateur n'est pas proposé ; un illisible l'est, avec son
// erreur.
func TestPresetsAreReadyToFillTheForm(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"mien.json":  `{"name":"Le mien","system":"rpcs3","values":{"maxSpursThreads":3,"spuBlockSize":"Tera","disparu":"1"}}`,
		"autre.json": `{"name":"Autre","system":"Un autre émulateur","values":{}}`,
		"casse.json": `{"name":`,
	} {
		if err := os.WriteFile(filepath.Join(store.presetsDir("ps3"), name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	presets := map[string]Preset{}
	list := store.Describe(rpcs3, "").Presets
	for _, p := range list {
		presets[p.ID] = p
	}
	if list[0].ID != "default" {
		t.Fatalf("le préréglage par défaut d'abord : %s", list[0].ID)
	}
	if _, listed := presets["autre"]; listed {
		t.Fatal("le préréglage d'un autre émulateur est proposé")
	}
	if presets["casse"].Error == "" {
		t.Fatal("un préréglage illisible doit le dire")
	}
	mien := presets["mien"]
	if mien.Values["maxSpursThreads"] != "3" || len(mien.Values) != 1 || strings.Join(mien.Ignored, ",") != "disparu,SPU block size" {
		t.Fatalf("le mien : %+v", mien)
	}
	if performance := presets["performance"]; performance.Values["resolutionScale"] != "50" || performance.Values["relaxedZcull"] != true || len(performance.Ignored) != 0 {
		t.Fatalf("performance : %+v", performance)
	}
}

// « SAVE AS PRESET » (le porteur, 2026-10-12) : les valeurs enregistrées sont
// gardées aussi en préréglage, sous le slug de leur nom ; le même nom remplace
// le préréglage de l'admin, et ne réécrit rien s'il dit déjà la même chose.
// Les préréglages LIVRÉS sont en lecture seule : leur nom est refusé à
// l'enregistrement comme à la suppression, et un refus n'écrit rien, pas même
// le fichier de l'émulateur.
func TestPresetsOfTheAdminAreSavedAndDeleted(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}

	saved, err := store.Save(rpcs3, "PlayStation 3", nil, map[string]any{"spuBlockSize": "Giga"}, "  Mon réglage S25 ! ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SavedPreset != "mon-reglage-s25" {
		t.Fatalf("préréglage enregistré : %q", saved.SavedPreset)
	}
	presets := map[string]Preset{}
	for _, p := range saved.Presets {
		presets[p.ID] = p
	}
	mine := presets["mon-reglage-s25"]
	if mine.Name != "Mon réglage S25 !" || !mine.Override || mine.Values["spuBlockSize"] != "Giga" || len(mine.Values) != len(saved.Params) {
		t.Fatalf("le mien : %+v", mine)
	}
	if presets["performance"].Override || presets["default"].Override {
		t.Fatal("les préréglages livrés doivent se dire en lecture seule")
	}
	written, _ := os.ReadFile(filepath.Join(store.presetsDir("ps3"), "mon-reglage-s25.json"))
	if !strings.HasPrefix(string(written), "{\n  \"name\": \"Mon réglage S25 !\",\n  \"override\": true,\n  \"system\": \"RPCS3\",\n  \"values\": {\n    \"resolutionScale\"") {
		t.Fatalf("préréglage écrit :\n%s", written)
	}

	// Le même nom remplace le sien.
	if _, err := store.Save(rpcs3, "", nil, map[string]any{"spuBlockSize": "Mega"}, "Mon réglage S25 !", nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range store.Describe(rpcs3, "").Presets {
		if p.ID == "mon-reglage-s25" && p.Values["spuBlockSize"] != "Mega" {
			t.Fatalf("le préréglage n'a pas été remplacé : %+v", p)
		}
	}

	// Les livrés, ni remplacés ni supprimés ; et rien n'est écrit.
	before, _ := os.ReadFile(store.emulatorFile(rpcs3))
	for _, name := range []string{"Performance", "default", "!!!"} {
		var refused *RefusedError
		if _, err := store.Save(rpcs3, "", nil, map[string]any{"spuBlockSize": "Giga"}, name, nil); !errors.As(err, &refused) {
			t.Errorf("enregistrer sous « %s » : %v, attendu un refus", name, err)
		}
	}
	if after, _ := os.ReadFile(store.emulatorFile(rpcs3)); string(after) != string(before) {
		t.Fatal("un refus de nom a quand même écrit le fichier de l'émulateur")
	}
	var refused *RefusedError
	if _, err := store.DeletePreset(rpcs3, "", "performance"); !errors.As(err, &refused) {
		t.Errorf("supprimer un préréglage livré : %v, attendu un refus", err)
	}
	for _, id := range []string{"inconnu", "../rpcs3", ""} {
		if _, err := store.DeletePreset(rpcs3, "", id); !errors.Is(err, ErrNoPreset) {
			t.Errorf("supprimer « %s » : %v, attendu ErrNoPreset", id, err)
		}
	}

	// Celui de l'admin se supprime.
	after, err := store.DeletePreset(rpcs3, "", "mon-reglage-s25")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range after.Presets {
		if p.ID == "mon-reglage-s25" {
			t.Fatal("le préréglage supprimé est encore proposé")
		}
	}
	if _, err := os.Stat(store.emulatorFile(rpcs3)); err != nil {
		t.Fatal("supprimer un préréglage a touché au fichier de l'émulateur")
	}

	// LE FICHIER DÉCIDE : un préréglage écrit à la main avec `"override":
	// false` est protégé comme ceux de GameFlex ; sans l'attribut, il est à
	// l'admin.
	locked := filepath.Join(store.presetsDir("ps3"), "fige.json")
	if err := os.WriteFile(locked, []byte(`{"name":"Figé","override":false,"system":"RPCS3","values":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(rpcs3, "", nil, nil, "Figé", nil); !errors.As(err, &refused) {
		t.Errorf("écraser un préréglage « override: false » : %v, attendu un refus", err)
	}
	if _, err := store.DeletePreset(rpcs3, "", "fige"); !errors.As(err, &refused) {
		t.Errorf("supprimer un préréglage « override: false » : %v, attendu un refus", err)
	}
	free := filepath.Join(store.presetsDir("ps3"), "libre.json")
	if err := os.WriteFile(free, []byte(`{"name":"Libre","system":"RPCS3","values":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeletePreset(rpcs3, "", "libre"); err != nil {
		t.Errorf("supprimer un préréglage sans l'attribut : %v", err)
	}

	// UN AUTRE ÉMULATEUR de la même console a son préréglage sous ce nom : il
	// ne s'écrase pas.
	other := filepath.Join(store.presetsDir("ps3"), "partage.json")
	if err := os.WriteFile(other, []byte(`{"name":"Partagé","system":"Un autre émulateur","values":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(rpcs3, "", nil, nil, "Partagé", nil); !errors.As(err, &refused) {
		t.Errorf("écraser le préréglage d'un autre émulateur : %v, attendu un refus", err)
	}
	if _, err := store.DeletePreset(rpcs3, "", "partage"); !errors.Is(err, ErrNoPreset) {
		t.Errorf("supprimer le préréglage d'un autre émulateur : %v, attendu ErrNoPreset", err)
	}
}

// LE SLUG — un nom de fichier : minuscules, lettres et chiffres, le reste en un
// seul tiret, les accents rendus à leur lettre.
func TestSlug(t *testing.T) {
	for name, want := range map[string]string{
		"S25, trial 1: SPU block size Giga": "s25-trial-1-spu-block-size-giga",
		"  Mon réglage S25 ! ":              "mon-reglage-s25",
		"Œuvre & Cœur":                      "oeuvre-coeur",
		"---":                               "",
		"../rpcs3":                          "rpcs3",
		strings.Repeat("a", 70):             strings.Repeat("a", 64),
	} {
		if got := Slug(name); got != want {
			t.Errorf("Slug(%q) = %q, attendu %q", name, got, want)
		}
	}
}

// CE QUE LE LANCEMENT REÇOIT : les adresses et les valeurs dans l'ordre du
// fichier quand il est activé ; rien quand il est désactivé ; une erreur quand
// il ne se lit pas. Écrit à la main, sans `enabled`, il s'applique puisqu'il
// existe, et un nombre ou un booléen sans guillemets valent leur valeur.
func TestSettingsFollowTheFile(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	settings := store.Settings(rpcs3)
	if settings.State != "enabled" || settings.Emulator != "rpcs3" || settings.Entries[0] != (Entry{Config: "Video > Resolution Scale", Value: "auto"}) {
		t.Fatalf("activé : %+v", settings)
	}
	for _, entry := range settings.Entries {
		if entry.Config == "Video > Multithreaded RSX" && entry.Value != "true" {
			t.Fatalf("un interrupteur arrive en texte « true » : %+v", entry)
		}
	}

	encoded, _ := encode(readRpcs3(t, store), false)
	_ = os.WriteFile(store.emulatorFile(rpcs3), encoded, 0o644)
	if settings := store.Settings(rpcs3); settings.State != "disabled" || len(settings.Entries) != 0 {
		t.Fatalf("désactivé : %+v", settings)
	}

	_ = os.WriteFile(store.emulatorFile(rpcs3), []byte(`{`), 0o644)
	if settings := store.Settings(rpcs3); settings.State != "error" || !strings.Contains(settings.Error, "unreadable") {
		t.Fatalf("illisible : %+v", settings)
	}

	_ = os.WriteFile(store.emulatorFile(rpcs3), []byte(`{
  "name": "Le mien", "system": "RPCS3",
  "params": {
    "threads": { "type": "select", "label": "Fils", "config": "Core > Max SPURS Threads", "value": 4,
                 "options": { "6": "6", "4": "4" } },
    "overlay": { "type": "switch", "label": "Compteur", "config": "Video>Performance Overlay>Enabled", "value": "true" },
    "cpu": { "type": "text", "label": "Processeur LLVM", "config": "Core > Use LLVM CPU", "value": "cortex-a720" }
  }
}`), 0o644)
	settings = store.Settings(rpcs3)
	want := []Entry{
		{Config: "Core > Max SPURS Threads", Value: "4"},
		{Config: "Video > Performance Overlay > Enabled", Value: "true"},
		{Config: "Core > Use LLVM CPU", Value: "cortex-a720"},
	}
	if settings.State != "enabled" || len(settings.Entries) != 3 {
		t.Fatalf("écrit à la main : %+v", settings)
	}
	for i := range want {
		if settings.Entries[i] != want[i] {
			t.Fatalf("écrit à la main, entrée %d : %+v", i, settings.Entries[i])
		}
	}
	if item := store.Describe(rpcs3, ""); !item.Enabled || item.Params[2].ID != "cpu" || item.Params[2].Type != "text" {
		t.Fatalf("le paramètre ajouté à la main n'est pas proposé : %+v", item.Params)
	}
}

// LES TEXTES LIVRÉS SONT EN ANGLAIS, et un fichier posé par une version
// précédente — libellés en français — montre quand même les textes livrés
// pour les paramètres livrés : plus de mélange des deux langues (relevé par le
// porteur). Un paramètre ajouté à la main garde les siens, et afficher un
// fichier ne le réécrit pas.
func TestTheShippedTextsAreEnglish(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	item := store.Describe(rpcs3, "")
	if item.Params[0].Label != "Render resolution" || item.Params[0].Options[0].Label != "Follow the stream" {
		t.Fatalf("textes livrés : %q %q", item.Params[0].Label, item.Params[0].Options[0].Label)
	}
	for _, preset := range item.Presets {
		if preset.ID == "default" && preset.Name != "Default" {
			t.Fatalf("préréglage par défaut : %q", preset.Name)
		}
	}

	old := []byte(`{"name":"PlayStation 3","system":"RPCS3","params":{
	  "resolutionScale":{"type":"select","label":"Résolution de rendu","config":"Video > Resolution Scale","value":"auto",
	    "options":{"auto":"Suivre le flux","50":"50 %"}},
	  "mine":{"type":"text","label":"Le mien","config":"Core > Use LLVM CPU","value":"x"}}}`)
	if err := os.WriteFile(store.emulatorFile(rpcs3), old, 0o644); err != nil {
		t.Fatal(err)
	}
	item = store.Describe(rpcs3, "")
	if item.Params[0].Label != "Render resolution" || item.Params[0].Options[0].Label != "Follow the stream" || item.Params[1].Label != "Le mien" {
		t.Fatalf("fichier ancien : %q %q %q", item.Params[0].Label, item.Params[0].Options[0].Label, item.Params[1].Label)
	}
	if again, _ := os.ReadFile(store.emulatorFile(rpcs3)); string(again) != string(old) {
		t.Fatal("afficher un fichier ne doit pas le réécrire")
	}
}

// CE QU'UN AUTRE SERVEUR PEUT EMPRUNTER (chantier 15) : les valeurs choisies
// et les préréglages perso — jamais ceux que GameFlex livre, qui sont les
// mêmes partout. Un émulateur sans rien de propre n'apparaît pas.
func TestTheInventoryListsWhatIsOwnHere(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if inventory := store.Inventory(); len(inventory) != 0 {
		t.Fatalf("rien de propre encore : %+v", inventory)
	}

	if _, err := store.Save(rpcs3, "PlayStation 3", nil, map[string]any{"spuBlockSize": "Giga"}, "Mon réglage", nil); err != nil {
		t.Fatal(err)
	}
	inventory := store.Inventory()
	if len(inventory) != 1 || inventory[0].Ref != rpcs3 || !inventory[0].Values ||
		len(inventory[0].Presets) != 1 || inventory[0].Presets[0].ID != "mon-reglage" {
		t.Fatalf("inventaire = %+v", inventory)
	}
}

// CE QU'UN ÉMULATEUR PRÊTE À UN AMI (chantier 15) : ses valeurs choisies, un
// préréglage perso — jamais un préréglage de GameFlex, ni des valeurs qui
// n'ont pas été choisies ici.
func TestLendOnlyWhatIsOwnHere(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lend(rpcs3, ""); !errors.Is(err, ErrNoPreset) {
		t.Fatalf("des valeurs livrées prêtées : %v", err)
	}
	if _, err := store.Lend(rpcs3, "default"); !errors.Is(err, ErrNoPreset) {
		t.Fatalf("un préréglage de GameFlex prêté : %v", err)
	}

	if _, err := store.Save(rpcs3, "PlayStation 3", nil, map[string]any{"spuBlockSize": "Giga"}, "Mon réglage", nil); err != nil {
		t.Fatal(err)
	}
	values, err := store.Lend(rpcs3, "")
	if err != nil || values.Values["spuBlockSize"] != "Giga" || values.System == "" {
		t.Fatalf("valeurs prêtées = %+v (%v)", values, err)
	}
	preset, err := store.Lend(rpcs3, "mon-reglage")
	if err != nil || preset.Name != "Mon réglage" || preset.Values["spuBlockSize"] != "Giga" {
		t.Fatalf("préréglage prêté = %+v (%v)", preset, err)
	}
}

// CE QU'ON IMPORTE D'UN AMI devient un préréglage perso, sous le nom qu'on lui
// donne (`<serveur>-<préréglage>`) : jamais par-dessus un autre, et seulement
// les valeurs que les paramètres d'ici acceptent.
func TestImportNeverOverwritesAndKeepsWhatFits(t *testing.T) {
	store := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}

	imported, ignored, err := store.Import(rpcs3, "PlayStation 3", "banc-release-Mon réglage",
		map[string]any{"spuBlockSize": "Giga", "inconnu": 3, "frameLimit": "nimporte-quoi"})
	if err != nil || imported.SavedPreset != "banc-release-mon-reglage" {
		t.Fatalf("import = %+v (%v)", imported.SavedPreset, err)
	}
	if strings.Join(ignored, ",") != "frameLimit,inconnu" {
		t.Fatalf("écartés = %v", ignored)
	}
	var found *Preset
	for index := range imported.Presets {
		if imported.Presets[index].ID == "banc-release-mon-reglage" {
			found = &imported.Presets[index]
		}
	}
	if found == nil || !found.Override || found.Values["spuBlockSize"] != "Giga" || len(found.Values) != 1 {
		t.Fatalf("préréglage importé = %+v", found)
	}

	if _, _, err := store.Import(rpcs3, "PlayStation 3", "banc-release-Mon réglage", map[string]any{"spuBlockSize": "Mega"}); !errors.Is(err, ErrExists) {
		t.Fatalf("un second import du même nom = %v, attendu ErrExists", err)
	}
	var refused *RefusedError
	if _, _, err := store.Import(rpcs3, "PlayStation 3", "default", map[string]any{"spuBlockSize": "Giga"}); !errors.As(err, &refused) {
		t.Fatalf("un import sous le nom d'un préréglage de GameFlex = %v", err)
	}
	if _, _, err := store.Import(rpcs3, "PlayStation 3", "rien-qui-aille", map[string]any{"inconnu": 1}); !errors.As(err, &refused) {
		t.Fatalf("un import sans valeur qui aille = %v", err)
	}
}

// UN PRÊT, VU D'ICI : sous quel nom il entrerait, et s'il y est déjà.
func TestOfferSaysWhereALoanWouldLand(t *testing.T) {
	store := Store{Root: t.TempDir()}
	offer := store.OfferOf("banc-release", "ps3", "rpcs3", "hd-perso", "HD perso")
	if offer.As != "banc-release-hd-perso" || offer.Installed || offer.Name != "HD perso" {
		t.Fatalf("prêt = %+v", offer)
	}
	if _, _, err := store.Import(rpcs3, "PlayStation 3", ImportName("banc-release", "HD perso"), map[string]any{"spuBlockSize": "Giga"}); err != nil {
		t.Fatal(err)
	}
	if offer := store.OfferOf("banc-release", "ps3", "rpcs3", "hd-perso", "HD perso"); !offer.Installed {
		t.Fatalf("après l'import : %+v", offer)
	}
	if values := store.OfferOf("banc-release", "ps3", "rpcs3", "", ""); values.As != "banc-release" || values.Name != "rpcs3" {
		t.Fatalf("ses valeurs = %+v", values)
	}
}

// CE QUE GAMEFLEX NE LIVRE PLUS PART : le fichier et les préréglages livrés
// d'un émulateur retiré (Eden le 2026-10-16, Citron le 2026-10-22 — tous deux revenus ; un nom fictif ici). Ceux de l'admin et d'un ami
// restent, comme le fichier d'un émulateur toujours livré.
func TestEnsurePrunesWhatIsNoLongerShipped(t *testing.T) {
	store := Store{Root: t.TempDir()}
	write := func(relative, content string) string {
		target := filepath.Join(store.Root, "params", relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		return target
	}
	retiredFile := write("switch/yuzu.json", `{"enabled": true}`)
	retiredPreset := write("switch/presets/yuzu-default.json", `{"name": "Default", "override": false, "system": "Yuzu", "values": {}}`)
	admin := write("switch/presets/yuzu-mine.json", `{"name": "Mine", "override": true, "system": "Yuzu", "values": {}}`)
	friend := write("switch/presets/ami-yuzu.json", `{"name": "Ami", "system": "Yuzu", "values": {}}`)
	kept := write("switch/eden.json", `{"enabled": true}`)

	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{retiredFile, retiredPreset} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s devait partir", gone)
		}
	}
	for _, stays := range []string{admin, friend, kept, filepath.Join(store.presetsDir("switch"), "default.json")} {
		if _, err := os.Stat(stays); err != nil {
			t.Errorf("%s devait rester : %v", stays, err)
		}
	}
}

// UN FICHIER ANCIEN REÇOIT CE QUE GAMEFLEX LIVRE DE PLUS (#388, 2026-10-19) :
// les paramètres livrés après lui, à leur valeur livrée et à leur place ; les
// options livrées qui manquent à ses listes. Ce qu'il porte ne bouge pas —
// une valeur changée, un paramètre ajouté à la main —, une adresse déjà
// réglée sous un autre nom n'est pas doublée, et un fichier complet n'est
// pas réécrit.
func TestEnsureCompletesAnOlderFile(t *testing.T) {
	phone := Store{Root: t.TempDir(), SeedOnFirstRun: true}
	_, shippedContent := shippedDefinition(rpcs3)
	var older map[string]any
	if err := json.Unmarshal(shippedContent, &older); err != nil {
		t.Fatal(err)
	}
	params := older["params"].(map[string]any)
	frameLimit := params["frameLimit"].(map[string]any)
	delete(frameLimit["options"].(map[string]any), "PS3 Native")
	frameLimit["value"] = "60"
	// Rejoué par une map : l'ordre du fichier ancien est alphabétique, et
	// seul l'ordre livré dit où vont les nouveaux.
	delete(params, "turnipDebug")
	delete(params, "ir3Debug")
	delete(params, "disableVertexCache")
	// Une adresse livrée, déjà réglée à la main sous un autre nom.
	delete(params, "strictRendering")
	params["mine"] = map[string]any{"type": "switch", "label": "Mine", "config": "Video > Strict Rendering Mode", "value": true}
	content, _ := json.Marshal(older)
	if err := os.MkdirAll(filepath.Dir(phone.emulatorFile(rpcs3)), 0o755); err != nil {
		t.Fatal(err)
	}
	// presets/ existe : ce n'est plus le premier démarrage.
	if err := os.MkdirAll(phone.presetsDir("ps3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(phone.emulatorFile(rpcs3), content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := phone.Ensure(); err != nil {
		t.Fatal(err)
	}
	f := readRpcs3(t, phone)
	if p, ok := f.Params.get("turnipDebug"); !ok || p.Value != "auto" || p.Config != "Environment > TU_DEBUG" {
		t.Fatalf("turnipDebug : %+v", p)
	}
	if p, ok := f.Params.get("disableVertexCache"); !ok || p.Value != false {
		t.Fatalf("disableVertexCache : %+v", p)
	}
	at := func(id string) int { return slices.Index(f.Params.keys, id) }
	if at("disableVertexCache") != at("disableZcullQueries")+1 || at("ir3Debug") != at("turnipDebug")+1 {
		t.Fatalf("l'ordre livré n'est pas suivi : %v", f.Params.keys)
	}
	if _, doubled := f.Params.get("strictRendering"); doubled {
		t.Fatal("une adresse déjà réglée a été doublée")
	}
	if p, _ := f.Params.get("mine"); p.Value != true {
		t.Fatalf("le paramètre ajouté à la main : %+v", p)
	}
	frame, _ := f.Params.get("frameLimit")
	if frame.Value != "60" || slices.Index(frame.Options.keys, "PS3 Native") != slices.Index(frame.Options.keys, "60")+1 {
		t.Fatalf("frameLimit : %v %v", frame.Value, frame.Options.keys)
	}

	completed, _ := os.ReadFile(phone.emulatorFile(rpcs3))
	stamp := time.Now().Add(-time.Hour)
	_ = os.Chtimes(phone.emulatorFile(rpcs3), stamp, stamp)
	if err := phone.Ensure(); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(phone.emulatorFile(rpcs3)); !info.ModTime().Equal(stamp) {
		t.Fatal("un fichier complet a été réécrit")
	}
	if again, _ := os.ReadFile(phone.emulatorFile(rpcs3)); string(again) != string(completed) {
		t.Fatal("le fichier a changé au second démarrage")
	}
}
