package domains

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	kernel := Service{Service: "kernel", Target: "KERNEL", Domains: []string{"health", "sessions"}}
	credential := Service{Service: "credential", Target: "CREDENTIAL", Domains: []string{"auth"}}

	cases := map[string]struct {
		services []Service
		refusal  string
	}{
		"une table saine":        {services: []Service{kernel, credential}},
		"une table vide":         {refusal: "aucun service"},
		"un domaine en double":   {services: []Service{kernel, {Service: "storage", Target: "STORAGE", Domains: []string{"sessions"}}}, refusal: `"sessions" est déclaré par kernel et par storage`},
		"un domaine avec barre":  {services: []Service{{Service: "storage", Target: "STORAGE", Domains: []string{"storage/admin"}}}, refusal: "domaine invalide"},
		"une variable manquante": {services: []Service{{Service: "storage", Domains: []string{"storage"}}}, refusal: "entrée incomplète"},
		"aucun domaine":          {services: []Service{{Service: "storage", Target: "STORAGE"}}, refusal: "entrée incomplète"},
	}
	for name, c := range cases {
		err := Validate(c.services)
		switch {
		case c.refusal == "" && err != nil:
			t.Errorf("%s : refusée (%v)", name, err)
		case c.refusal != "" && (err == nil || !strings.Contains(err.Error(), c.refusal)):
			t.Errorf("%s : %v, attendu un refus contenant %q", name, err, c.refusal)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`[{"service":"kernel","target":"KERNEL","websocket":true,"domains":["health"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	services, err := Load(good)
	if err != nil || len(services) != 1 || !services[0].WebSocket || services[0].Domains[0] != "health" {
		t.Errorf("Load(good) = %+v, %v", services, err)
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte(`[{"service":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(broken); err == nil {
		t.Error("Load(broken) : aucune erreur")
	}

	if _, err := Load(filepath.Join(dir, "absent.json")); err == nil {
		t.Error("Load(absent) : aucune erreur")
	}
}
