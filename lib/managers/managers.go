// Package managers audits packages installed by system package managers
// (go install, brew, ...): what is installed, where it came from, and
// whether its integrity can be verified. It is the "system" half of
// doctor --audit; lib/audit owns the report schema.
//
// Adapters report only what the underlying manager can actually prove.
// A manager that keeps no integrity data yields IntegrityNotVerifiable
// rather than a silent pass.
package managers

import (
	"context"

	"github.com/sid-technologies/scuta/lib/audit"
)

// Manager audits the packages installed by one package manager.
type Manager interface {
	// Name is the manager identifier used in reports ("go", "brew", ...).
	Name() string
	// Detect reports whether this manager is present on the machine.
	Detect() bool
	// Audit inventories and checks the manager's packages. Only called
	// when Detect returned true.
	Audit(ctx context.Context) audit.ManagerReport
}

// All returns every supported manager.
func All() []Manager {
	return []Manager{NewGoBin(), NewBrew(), NewMise(), NewDpkg()}
}

// Collect runs each manager and assembles the system section of the audit
// report. Undetected managers are still listed, so the report is explicit
// about what was and was not checked.
func Collect(ctx context.Context, mgrs []Manager) *audit.System {
	sys := &audit.System{}
	for _, m := range mgrs {
		if !m.Detect() {
			sys.Managers = append(sys.Managers, audit.ManagerReport{Name: m.Name()})
			continue
		}
		sys.Managers = append(sys.Managers, m.Audit(ctx))
	}
	return sys
}
