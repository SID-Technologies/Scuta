package manifest

import (
	"sort"
	"strings"
)

// ActionType is the kind of reconciliation step needed for a tool.
type ActionType int

const (
	// ActionInstall installs a tool that is required but not present.
	ActionInstall ActionType = iota
	// ActionChange installs a different (pinned) version than what is present.
	ActionChange
	// ActionRemove uninstalls a tool not in the manifest (only with prune).
	ActionRemove
	// ActionUpToDate means the tool already satisfies the manifest.
	ActionUpToDate
)

func (a ActionType) String() string {
	switch a {
	case ActionInstall:
		return "install"
	case ActionChange:
		return "change"
	case ActionRemove:
		return "remove"
	default:
		return "up-to-date"
	}
}

// Action is a single reconciliation step.
type Action struct {
	Name           string
	Type           ActionType
	CurrentVersion string // installed version ("" if not installed)
	TargetVersion  string // desired version ("" means latest)
	IsLatest       bool   // true when the manifest requested "latest"/unpinned
	Repo           string // repo override from the manifest entry (may be "")
	Bin            string // binary name override from the manifest entry (may be "")
}

// Plan computes the reconciliation steps needed to make `installed` match the
// manifest. `installed` maps tool name -> currently installed version.
//
// A tool pinned to an explicit version that differs from what is installed
// yields ActionChange. A tool pinned to "latest" is only installed when
// missing; keeping it current is left to `scuta update` (the manifest can't
// know the newest upstream version without a network call, and sync stays
// deterministic/offline-friendly). With prune=true, installed tools absent
// from the manifest yield ActionRemove.
func (m *Manifest) Plan(installed map[string]string, prune bool) []Action {
	var actions []Action

	for _, name := range m.Names() {
		entry := m.Tools[name]
		target, isLatest := NormalizeVersion(entry.Version)

		current, isInstalled := installed[name]
		currentNorm := strings.TrimPrefix(current, "v")

		action := Action{
			Name:           name,
			CurrentVersion: current,
			TargetVersion:  target,
			IsLatest:       isLatest,
			Repo:           entry.Repo,
			Bin:            entry.Bin,
		}

		if !isInstalled {
			action.Type = ActionInstall
			actions = append(actions, action)
			continue
		}

		if !isLatest && target != currentNorm {
			action.Type = ActionChange
			actions = append(actions, action)
			continue
		}

		action.Type = ActionUpToDate
		actions = append(actions, action)
	}

	if prune {
		var extra []string
		for name := range installed {
			if _, inManifest := m.Tools[name]; !inManifest {
				extra = append(extra, name)
			}
		}
		sort.Strings(extra)
		for _, name := range extra {
			actions = append(actions, Action{
				Name:           name,
				Type:           ActionRemove,
				CurrentVersion: installed[name],
			})
		}
	}

	return actions
}

// Changes returns only the actions that require work (everything except
// up-to-date entries).
func Changes(actions []Action) []Action {
	var changes []Action
	for _, a := range actions {
		if a.Type != ActionUpToDate {
			changes = append(changes, a)
		}
	}
	return changes
}
