package manifest

import "testing"

func findAction(actions []Action, name string) (Action, bool) {
	for _, a := range actions {
		if a.Name == name {
			return a, true
		}
	}
	return Action{}, false
}

func TestPlan_InstallChangeUpToDate(t *testing.T) {
	m := &Manifest{Tools: map[string]Entry{
		"pilum":   {Version: "0.7.5"},
		"ripgrep": {Version: "15.2.0"},
		"bat":     {Version: "0.24.0"},
		"fzf":     {Version: "latest"},
	}}
	installed := map[string]string{
		"pilum": "0.7.0",
		"bat":   "0.24.0",
		"fzf":   "0.74.1",
	}

	actions := m.Plan(installed, false)

	if a, _ := findAction(actions, "pilum"); a.Type != ActionChange {
		t.Errorf("pilum: expected change, got %s", a.Type)
	}
	if a, _ := findAction(actions, "ripgrep"); a.Type != ActionInstall {
		t.Errorf("ripgrep: expected install, got %s", a.Type)
	}
	if a, _ := findAction(actions, "bat"); a.Type != ActionUpToDate {
		t.Errorf("bat: expected up-to-date, got %s", a.Type)
	}
	if a, _ := findAction(actions, "fzf"); a.Type != ActionUpToDate {
		t.Errorf("fzf (latest, installed): expected up-to-date, got %s", a.Type)
	}
}

func TestPlan_VersionPrefixInsensitive(t *testing.T) {
	m := &Manifest{Tools: map[string]Entry{"bat": {Version: "v0.24.0"}}}
	installed := map[string]string{"bat": "0.24.0"}

	actions := m.Plan(installed, false)
	if a, _ := findAction(actions, "bat"); a.Type != ActionUpToDate {
		t.Errorf("expected v-prefix to be treated as equal, got %s", a.Type)
	}
}

func TestPlan_PruneRemovesUnlisted(t *testing.T) {
	m := &Manifest{Tools: map[string]Entry{"pilum": {Version: "latest"}}}
	installed := map[string]string{"pilum": "0.7.5", "orphan": "1.0.0"}

	withoutPrune := Changes(m.Plan(installed, false))
	if len(withoutPrune) != 0 {
		t.Errorf("without prune: expected no changes, got %d", len(withoutPrune))
	}

	withPrune := m.Plan(installed, true)
	a, ok := findAction(withPrune, "orphan")
	if !ok || a.Type != ActionRemove {
		t.Errorf("with prune: expected orphan to be removed, got %+v (ok=%v)", a, ok)
	}
}

func TestPlan_PropagatesRepoAndBin(t *testing.T) {
	m := &Manifest{Tools: map[string]Entry{
		"ripgrep": {Version: "14.1.0", Repo: "BurntSushi/ripgrep", Bin: "rg"},
	}}

	actions := m.Plan(map[string]string{}, false)
	a, ok := findAction(actions, "ripgrep")
	if !ok {
		t.Fatal("expected a ripgrep action")
	}
	if a.Repo != "BurntSushi/ripgrep" {
		t.Errorf("expected repo propagated, got %q", a.Repo)
	}
	if a.Bin != "rg" {
		t.Errorf("expected bin propagated, got %q", a.Bin)
	}
}
