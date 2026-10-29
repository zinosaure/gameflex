package storage_test

import (
	"os/exec"
	"strings"
	"testing"
)

// La bibliothèque de jeux doit tourner AUX DEUX ENDROITS : dans le kernel, et
// dans l'application Android via gomobile. C'est toute la raison de l'avoir
// écrite en Go plutôt qu'en Kotlin — une seule implémentation du scan, des
// alias de dossiers et des règles d'extension.
//
// Comme pour shared/session, la propriété est tenue par un test plutôt que par
// un paragraphe : un import système ici, et l'application mobile perd sa
// bibliothèque des mois avant que quiconque ne rouvre le sujet.
func TestScannerStillCompilesForAndroid(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("pas de chaîne Go ici — rien avec quoi compiler")
	}

	build := exec.Command("go", "build", "./...")
	build.Env = append(build.Environ(), "CGO_ENABLED=0", "GOOS=android", "GOARCH=arm64")

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"la bibliothèque de jeux ne compile plus pour android/arm64 sans cgo.\n"+
				"Quelque chose de spécifique au système a atteint ce paquet.\n\n%s",
			strings.TrimSpace(string(output)),
		)
	}
}
