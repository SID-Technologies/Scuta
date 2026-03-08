package workerqueue

// TaskInfo holds configuration for a tool operation task.
type TaskInfo struct {
	ToolName string // Name of the tool being operated on
	Version  string // Target version (empty for latest)
	Action   string // "install", "update", "uninstall"
	Timeout  int    // Timeout in seconds
	Retries  int    // Number of retries
	Force    bool   // Force reinstall
}

// NewTaskInfo creates a new TaskInfo with default values.
func NewTaskInfo(toolName, version, action string) *TaskInfo {
	return &TaskInfo{
		ToolName: toolName,
		Version:  version,
		Action:   action,
		Timeout:  300,
		Retries:  3,
	}
}
