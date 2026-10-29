package docker

import (
	"slices"
	"testing"

	"github.com/docker/docker/api/types/filters"

	"gameflex/kernel/internal/config"
)

// Une pile ne liste que ses parties : l'identifiant de session ET le nom de la
// pile, plus ce que l'appelant ajoute (les conteneurs sortis, pour le balayage).
func TestSessionFiltersKeepToTheirOwnStack(t *testing.T) {
	client := &Client{config: config.Config{StackName: "release"}}

	args := client.sessionFilters(filters.Arg("status", "exited"))

	labels := args.Get("label")
	if !slices.Contains(labels, labelSessionID) {
		t.Errorf("labels %v : il manque %q", labels, labelSessionID)
	}
	if !slices.Contains(labels, "gameflex.stack=release") {
		t.Errorf("labels %v : il manque gameflex.stack=release", labels)
	}
	if slices.Contains(labels, "gameflex.stack=gameflex") {
		t.Errorf("labels %v : la pile d'à côté passerait", labels)
	}
	if status := args.Get("status"); !slices.Equal(status, []string{"exited"}) {
		t.Errorf("status %v, attendu [exited]", status)
	}
}
