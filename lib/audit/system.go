package audit

// Integrity states for system packages. Adapters must report what they can
// actually prove; a manager with no integrity data says so instead of
// passing silently.
const (
	// IntegrityRecorded: the manager recorded verifiable origin metadata
	// (for example a module sum embedded at build time).
	IntegrityRecorded = "recorded"
	// IntegrityNotVerifiable: the manager keeps no integrity data for this
	// package, so tampering cannot be detected.
	IntegrityNotVerifiable = "not-verifiable"
	// IntegrityUnknown: the adapter could not determine integrity state.
	IntegrityUnknown = "unknown"
)

// SystemPackage is one package installed by a system package manager.
type SystemPackage struct {
	Name       string    `json:"name"`
	Version    string    `json:"version,omitempty"`
	Source     string    `json:"source,omitempty"`
	BinaryPath string    `json:"binary_path,omitempty"`
	Integrity  string    `json:"integrity"`
	Findings   []Finding `json:"findings,omitempty"`
}

// ManagerReport is the audit result for one package manager.
type ManagerReport struct {
	Name     string          `json:"name"`
	Detected bool            `json:"detected"`
	Packages []SystemPackage `json:"packages,omitempty"`
	Findings []Finding       `json:"findings,omitempty"`
}

// System is the machine-wide package audit, one entry per known manager.
type System struct {
	Managers []ManagerReport `json:"managers"`
}
