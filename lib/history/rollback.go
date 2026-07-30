package history

import "strings"

// PreviousVersion resolves the version a tool should roll back to, given
// history entries ordered most recent first (as returned by Load) and the
// currently installed version.
//
// It first skips forward to the most recent successful record that
// established the current version (so stale runs after the fact don't
// confuse resolution), then returns the first older successful version that
// differs from the current one. When the current version never appears in
// history, the first differing successful version wins.
func PreviousVersion(entries []Entry, tool, currentVersion string) (string, bool) {
	current := normalizeVersion(currentVersion)
	seenCurrent := false

	for _, e := range entries {
		for _, tr := range e.Tools {
			if tr.Name != tool || !tr.Success || tr.Version == "" {
				continue
			}
			if !isInstallAction(tr.Action) {
				continue
			}

			v := normalizeVersion(tr.Version)
			if v == current {
				seenCurrent = true
				continue
			}
			// Only trust versions recorded before the current one was
			// established — unless current never shows up at all.
			if seenCurrent || !containsVersion(entries, tool, current) {
				return tr.Version, true
			}
		}
	}

	return "", false
}

// containsVersion reports whether any successful record for tool carries the
// given normalized version.
func containsVersion(entries []Entry, tool, normalized string) bool {
	for _, e := range entries {
		for _, tr := range e.Tools {
			if tr.Name == tool && tr.Success && isInstallAction(tr.Action) &&
				normalizeVersion(tr.Version) == normalized {
				return true
			}
		}
	}
	return false
}

// isInstallAction reports whether the action left a version on disk.
func isInstallAction(action string) bool {
	switch action {
	case "install", "update", "rollback":
		return true
	}
	return false
}

// normalizeVersion strips a leading "v" so "v1.2.3" and "1.2.3" compare equal.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}
