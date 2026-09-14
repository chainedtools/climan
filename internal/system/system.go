// Package system provides OS-level helpers: path resolution, command
// execution, streaming output and downloads.
package system

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// Home returns the current user's home directory.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	return os.Getenv("HOME")
}

// Expand replaces a leading "~" with the home directory.
func Expand(p string) string {
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// homeFile returns the expanded path for a "~/..." style path if the file
// exists, otherwise "".
func homeFile(p string) string {
	full := Expand(p)
	if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
		return full
	}
	return ""
}

// HomeFile returns the expanded path for a "~/..." style path if the file
// exists, otherwise "". Exported for use by the registry package.
func HomeFile(p string) string { return homeFile(p) }

// LookPath wraps exec.LookPath, returning the absolute path.
func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// LookPathAny returns the first name on PATH, else an error.
func LookPathAny(names ...string) (string, error) {
	var errs []string
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		} else {
			errs = append(errs, fmt.Sprintf("%s: %v", n, err))
		}
	}
	return "", fmt.Errorf("none of %v found on PATH (%s)", names, strings.Join(errs, "; "))
}

// Runner executes commands and forwards stdout/stderr to w (which may be nil
// to discard output).
type Runner struct {
	// Out receives stdout+stderr streams. nil discards them.
	Out io.Writer
	// Env are extra KEY=VALUE entries appended to the environment.
	Env []string
}

// Run executes a command and waits for it, returning its combined output on
// success (even when Out is set) or an error containing the output on failure.
func (r *Runner) Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return r.RunContext(ctx, name, args...)
}

// RunContext is Run with a context.
func (r *Runner) RunContext(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), r.Env...)
	return r.run(cmd)
}

// RunBash executes a bash -c snippet.
func (r *Runner) RunBash(script string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return r.RunBashContext(ctx, script)
}

// RunBashContext is RunBash with a context.
func (r *Runner) RunBashContext(ctx context.Context, script string) (string, error) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return "", fmt.Errorf("bash is required: %w", err)
	}
	cmd := exec.CommandContext(ctx, bash, "-c", script)
	cmd.Env = append(os.Environ(), r.Env...)
	return r.run(cmd)
}

// run wires stdout/stderr to a pipe that fans out to Out (if set) and a
// capture buffer.
func (r *Runner) run(cmd *exec.Cmd) (string, error) {
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		tee := io.TeeReader(pr, &buf)
		if r.Out != nil {
			_, _ = io.Copy(r.Out, tee)
		} else {
			_, _ = io.Copy(io.Discard, tee)
		}
	}()

	err := cmd.Run()
	_ = pw.Close()
	<-done
	return buf.String(), err
}

// Download fetches url into dest (creating parent dirs) and returns the
// number of bytes written. Uses context-aware exec of curl or wget.
func Download(ctx context.Context, url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if curl, err := exec.LookPath("curl"); err == nil {
		cmd := exec.CommandContext(ctx, curl, "-fsSL", "--retry", "3", "-o", dest, url)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.Remove(dest)
			return fmt.Errorf("curl %s: %w: %s", url, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if wget, err := exec.LookPath("wget"); err == nil {
		cmd := exec.CommandContext(ctx, wget, "-q", "-O", dest, url)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.Remove(dest)
			return fmt.Errorf("wget %s: %w: %s", url, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return fmt.Errorf("neither curl nor wget is available")
}

// OS returns the lowercase GOOS for asset URLs.
func OS() string { return runtime.GOOS }

// Arch returns the lowercase GOARCH for asset URLs.
func Arch() string { return runtime.GOARCH }
