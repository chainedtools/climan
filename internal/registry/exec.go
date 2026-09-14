package registry

import (
	"go.solved.gg/climan/internal/system"
)

// lookPath resolves a command name on PATH.
func lookPath(name string) (string, error) {
	return system.LookPath(name)
}

// runCmd runs a command with args, returning combined output.
func runCmd(name string, args ...string) (string, error) {
	var r system.Runner
	return r.Run(name, args...)
}

// runBash runs a bash -c snippet, returning combined output.
func runBash(script string) (string, error) {
	var r system.Runner
	return r.RunBash(script)
}

// homeFile returns the expanded path for a "~/..." style path if the file
// exists, otherwise "".
func homeFile(p string) string {
	return system.HomeFile(p)
}
