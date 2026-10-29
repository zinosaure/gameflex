package sessionstatus

import (
	"os"
	"testing"
)

// Le compteur de son propre processus se lit ; celui d'un processus qui
// n'existe pas, non.
func TestReadBytesOfThisProcess(t *testing.T) {
	bytes, ok := ReadBytes(os.Getpid())
	if !ok {
		t.Fatal("/proc/self/io illisible pour son propre processus")
	}
	if bytes < 0 {
		t.Fatalf("read_bytes négatif : %d", bytes)
	}
	if _, ok := ReadBytes(-1); ok {
		t.Fatal("un processus qui n'existe pas a rendu une lecture")
	}
}
