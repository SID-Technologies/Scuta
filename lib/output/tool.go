package output

// ToolInfo holds all fields for JSON serialization across info, list, and
// registry commands. Fields are omitempty so each command only includes
// the fields it populates.
type ToolInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Source      string   `json:"source,omitempty"`
	Status      string   `json:"status,omitempty"`
	Version     string   `json:"version,omitempty"`
	Installed   string   `json:"installed,omitempty"`
	InstalledAt string   `json:"installed_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	BinaryPath  string   `json:"binary_path,omitempty"`
	BinarySize  string   `json:"binary_size,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// ConfigEntry holds a single config key-value pair for JSON output.
type ConfigEntry struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	DefaultValue string `json:"default"`
}
