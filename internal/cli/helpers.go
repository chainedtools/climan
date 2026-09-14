package cli

import (
	"os/exec"
	"runtime"
)

// lookPath resolves a command on PATH.
func lookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// osArch returns the lowercase GOOS/GOARCH pair.
func osArch() (string, string) {
	return runtime.GOOS, runtime.GOARCH
}
