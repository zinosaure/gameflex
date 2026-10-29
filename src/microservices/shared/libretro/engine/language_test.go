package engine

import (
	"testing"
	"unsafe"
)

// LA LANGUE DE L'APPLICATION, RÉPONDUE AUX CŒURS (chantier 21) : le français
// dans l'énumération de libretro (2), l'anglais (0) — et l'anglais encore
// quand la page n'a rien dit, comme avant.
func TestGetLanguageAnswersTheApplicationLanguage(t *testing.T) {
	for language, want := range map[string]uint32{"fr": 2, "en": 0, "": 0} {
		e := &Engine{config: Config{Language: language}}
		got := uint32(99)
		if !e.environment(39, unsafe.Pointer(&got)) || got != want {
			t.Errorf("GET_LANGUAGE pour %q = %d, attendu %d", language, got, want)
		}
	}
}

// Chaque option de langue a une valeur pour chaque langue de l'application :
// une langue oubliée laisserait le cœur dans la sienne sans rien dire.
func TestLanguageVariablesCoverTheApplicationLanguages(t *testing.T) {
	for key, values := range languageVariables {
		for language := range retroLanguages {
			if values[language] == "" {
				t.Errorf("%s n'a pas de valeur pour %q", key, language)
			}
		}
	}
}
