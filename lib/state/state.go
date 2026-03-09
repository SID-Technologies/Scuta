// Package state manages ~/.scuta/state.json — tracks installed tool versions
// and last update check timestamps.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
)

const stateFile = "state.json"

// ToolState tracks the installed version and metadata for a single tool.
type ToolState struct {
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	BinaryPath  string    `json:"binary_path"`
}

// CurrentStateVersion is the current state file format version.
const CurrentStateVersion = 1

// State represents the full Scuta state file.
type State struct {
	Version         int                  `json:"version,omitempty"`
	LastUpdateCheck time.Time            `json:"last_update_check"`
	Tools           map[string]ToolState `json:"tools"`
}

// NewState returns an empty state.
func NewState() *State {
	return &State{
		Version: CurrentStateVersion,
		Tools:   make(map[string]ToolState),
	}
}

// FilePath returns the state file path under the given scuta directory.
func FilePath(scutaDir string) string {
	return filepath.Join(scutaDir, stateFile)
}

// Load reads the state from disk. Returns empty state if file doesn't exist.
func Load(scutaDir string) (*State, error) {
	fp := FilePath(scutaDir)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, errors.Wrap(err, "reading state file")
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, errors.Wrap(err, "parsing state file")
	}

	if s.Tools == nil {
		s.Tools = make(map[string]ToolState)
	}

	// Auto-migrate pre-versioned state files
	if s.Version == 0 {
		s.Version = CurrentStateVersion
		_ = s.Save(scutaDir)
	}

	return &s, nil
}

// Save writes the state to disk.
func (s *State) Save(scutaDir string) error {
	if err := os.MkdirAll(scutaDir, 0o700); err != nil {
		return errors.Wrap(err, "creating scuta directory")
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshaling state")
	}

	fp := FilePath(scutaDir)
	if err := os.WriteFile(fp, data, 0o600); err != nil {
		return errors.Wrap(err, "writing state file")
	}

	return nil
}

// SetTool records a tool's installed state.
func (s *State) SetTool(name string, ts ToolState) {
	s.Tools[name] = ts
}

// RemoveTool removes a tool from the state.
func (s *State) RemoveTool(name string) {
	delete(s.Tools, name)
}

// GetTool returns a tool's state, or false if not installed.
func (s *State) GetTool(name string) (ToolState, bool) {
	ts, ok := s.Tools[name]
	return ts, ok
}
