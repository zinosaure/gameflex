package session

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Le démarrage part au kernel en UN envoi, sur la route de la partie, sous son secret (#751).
func TestReportBootPushesOnceToTheSessionRoute(t *testing.T) {
	var got []Boot
	var paths, secrets []string
	kernel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var boot Boot
		_ = json.NewDecoder(r.Body).Decode(&boot)
		got = append(got, boot)
		paths = append(paths, r.Method+" "+r.URL.Path)
		secrets = append(secrets, r.Header.Get("X-Internal-Secret"))
	}))
	defer kernel.Close()

	if err := ReportBoot(kernel.URL+"/", "s3cret", "abc", Boot{State: "booting", Unpacking: &BootUnpacking{Done: 5, Total: 10}}); err != nil {
		t.Fatalf("booting : %v", err)
	}
	if err := ReportBoot(kernel.URL, "s3cret", "abc", BootReady); err != nil {
		t.Fatalf("ready : %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("%d envois, deux attendus (un par état)", len(got))
	}
	if paths[0] != "POST /internal/sessions/abc/boot" || paths[1] != paths[0] {
		t.Errorf("routes : %v", paths)
	}
	if secrets[0] != "s3cret" {
		t.Errorf("secret : %q", secrets[0])
	}
	if got[0].State != "booting" || got[0].Unpacking == nil || got[0].Unpacking.Done != 5 || got[0].Unpacking.Total != 10 {
		t.Errorf("booting reçu : %+v", got[0])
	}
	if got[1].State != "ready" || got[1].Unpacking != nil {
		t.Errorf("ready reçu : %+v", got[1])
	}
}

// Un refus est une réponse : AnnounceReady ne le redit pas.
func TestAnnounceReadyDoesNotRepeatARefusal(t *testing.T) {
	calls := 0
	kernel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer kernel.Close()

	if AnnounceReady(kernel.URL, "faux", "abc") == nil {
		t.Error("un refus doit se dire")
	}
	if calls != 1 {
		t.Errorf("%d envois pour un refus, un seul attendu", calls)
	}
}

func TestReportBootRefusesWithoutKernel(t *testing.T) {
	if ReportBoot("", "s", "abc", BootReady) == nil {
		t.Error("sans adresse, l'envoi doit échouer, pas réussir en silence")
	}
}

func TestReportBootSaysWhenTheKernelRefuses(t *testing.T) {
	kernel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer kernel.Close()

	if ReportBoot(kernel.URL, "faux", "abc", BootReady) == nil {
		t.Error("un 401 du kernel doit se dire")
	}
}
