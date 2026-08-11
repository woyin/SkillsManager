package cmd

import (
	"fmt"

	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/wellknown"
)

// installMode names the mutually exclusive install workflows selected by the
// command line. Keeping this decision separate from execution makes the CLI
// contract directly testable without creating a Registry or touching disk.
type installMode uint8

const (
	profileInstallMode installMode = iota
	registryInstallMode
	directInstallMode
	lockRestoreMode
)

// classifyInstallRequest applies install's precedence rules. The deprecated
// --from-registry flag wins over --from-lock for backwards compatibility;
// without either flag, bare names select Registry Install and every other
// argument is a Direct Install source.
func classifyInstallRequest(args []string, fromRegistry, fromLock bool) (installMode, string, error) {
	if fromRegistry {
		if len(args) != 1 {
			return 0, "", fmt.Errorf("--from-registry requires exactly one skill name (or comma-separated names)")
		}
		return registryInstallMode, args[0], nil
	}
	if fromLock {
		return lockRestoreMode, "", nil
	}
	if len(args) == 0 {
		return profileInstallMode, "", nil
	}

	arg := args[0]
	if registry.IsBareName(arg) && !wellknown.IsSource(arg) {
		return registryInstallMode, arg, nil
	}
	return directInstallMode, arg, nil
}
