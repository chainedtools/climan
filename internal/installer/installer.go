// Package installer reconciles the system against the declarative manifest:
// it installs, updates and removes tools, and reports their status.
package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.solved.gg/climan/internal/registry"
	"go.solved.gg/climan/internal/system"
)

// State describes a tool's current condition.
type State string

const (
	StateMissing  State = "missing"
	StateReady    State = "ready"
	StateOutdated State = "outdated"
	StateUnknown  State = "unknown"
)

func (s State) Label() string {
	switch s {
	case StateMissing:
		return "not installed"
	case StateReady:
		return "installed"
	case StateOutdated:
		return "update available"
	default:
		return "unknown"
	}
}

// Status is the observed state of a single tool.
type Status struct {
	Tool      *registry.Tool
	State     State
	Path      string
	Version   string
	Desired   string
	Error     string
	InstallBy string // human description of the method that will be used
}

// Installer performs tool operations. It is safe to reuse across calls.
type Installer struct {
	// BinDir is where standalone binaries land (default ~/.local/bin).
	BinDir string
	// Out receives installer script output. nil discards it.
	Out io.Writer
	// DryRun prints what would run without executing.
	DryRun bool
	// HTTPClient is used for GitHub API/latest lookups.
	HTTPClient *http.Client

	reg *registry.Registry
}

// New creates an Installer with defaults.
func New() *Installer {
	return &Installer{
		BinDir:     filepath.Join(system.Home(), ".local", "bin"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		reg:        registry.New(),
	}
}

// Registry returns the tool catalog the installer uses.
func (i *Installer) Registry() *registry.Registry { return i.reg }

// runner returns a Runner wired to this installer's output.
func (i *Installer) runner() *system.Runner {
	return &system.Runner{Out: i.Out}
}

// logf writes a line to the output stream if set.
func (i *Installer) logf(format string, args ...any) {
	if i.Out != nil {
		_, _ = fmt.Fprintf(i.Out, format+"\n", args...)
	}
}

// Status inspects a single tool.
func (i *Installer) Status(t *registry.Tool, desired string) Status {
	st := Status{Tool: t, Desired: desired, State: StateMissing}
	path, ok := t.DetectPath()
	if !ok {
		// Binary-installed tools may live in BinDir rather than PATH.
		for _, m := range t.Install {
			if m.Kind != registry.MethodBinary {
				continue
			}
			cand := filepath.Join(i.BinDir, m.ExeIn)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
				path, ok = cand, true
				break
			}
		}
	}
	if !ok {
		st.InstallBy = i.describe(t)
		return st
	}
	st.Path = path
	st.State = StateReady
	if ver, err := t.VersionOf(path); err == nil {
		st.Version = strings.TrimSpace(ver)
	} else {
		st.State = StateUnknown
		st.Error = err.Error()
	}
	return st
}

// describe summarises the first install method for human output.
func (i *Installer) describe(t *registry.Tool) string {
	if len(t.Install) == 0 {
		return "no install method defined"
	}
	m := t.Install[0]
	switch m.Kind {
	case registry.MethodScript:
		return "official install script"
	case registry.MethodBinary:
		return fmt.Sprintf("GitHub release binary (%s)", m.Repo)
	case registry.MethodNPM:
		return fmt.Sprintf("npm package %s", m.NpmPackage)
	case registry.MethodGit:
		return fmt.Sprintf("git clone %s", m.GitRepo)
	default:
		return string(m.Kind)
	}
}

// InstallAll installs every tool in the manifest that is missing.
// Returns per-tool results.
func (i *Installer) InstallAll(ctx context.Context, tools []*registry.Tool, desired func(*registry.Tool) string) []Result {
	return i.runEach(ctx, tools, "install", func(t *registry.Tool) error {
		st := i.Status(t, desired(t))
		if st.State != StateMissing {
			return nil
		}
		return i.Install(ctx, t, desired(t))
	})
}

// UpdateAll updates every installed tool.
func (i *Installer) UpdateAll(ctx context.Context, tools []*registry.Tool) []Result {
	return i.runEach(ctx, tools, "update", func(t *registry.Tool) error {
		return i.Update(ctx, t)
	})
}

// Result captures the outcome of one tool operation.
type Result struct {
	Tool    *registry.Tool
	Op      string // "install", "update", "remove", "skip"
	Err     error
	Note    string
	Skipped bool
}

// runEach applies fn to every tool, short-circuiting nothing.
func (i *Installer) runEach(ctx context.Context, tools []*registry.Tool, op string, fn func(*registry.Tool) error) []Result {
	results := make([]Result, 0, len(tools))
	for _, t := range tools {
		res := Result{Tool: t, Op: op}
		if err := fn(t); err != nil {
			res.Err = err
		}
		results = append(results, res)
	}
	return results
}

// Install tries each install method in order until one succeeds.
func (i *Installer) Install(ctx context.Context, t *registry.Tool, version string) error {
	i.logf("==> installing %s (%s)", t.Name, orVersion(version))
	var errs []string
	for _, m := range t.Install {
		if err := i.apply(ctx, t, m, version); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.Kind, err))
			i.logf("    method %s failed: %v", m.Kind, err)
			continue
		}
		if m.Note != "" {
			i.logf("    %s", m.Note)
		}
		return nil
	}
	return fmt.Errorf("all install methods failed: %s", strings.Join(errs, "; "))
}

// Update tries each update method in order until one succeeds.
func (i *Installer) Update(ctx context.Context, t *registry.Tool) error {
	path, ok := t.DetectPath()
	if !ok {
		return fmt.Errorf("%s is not installed", t.Name)
	}
	i.logf("==> updating %s (%s)", t.Name, path)
	var errs []string
	for _, m := range t.UpdateMethods() {
		if err := i.apply(ctx, t, m, "latest"); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.Kind, err))
			i.logf("    method %s failed: %v", m.Kind, err)
			continue
		}
		if m.Note != "" {
			i.logf("    %s", m.Note)
		}
		return nil
	}
	return fmt.Errorf("all update methods failed: %s", strings.Join(errs, "; "))
}

// Remove uninstalls a tool best-effort: npm global uninstall, git dir
// removal, or deletion of the detected binary.
func (i *Installer) Remove(ctx context.Context, t *registry.Tool) error {
	i.logf("==> removing %s", t.Name)
	if i.DryRun {
		return nil
	}
	var errs []string
	removed := false
	for _, m := range t.Install {
		switch m.Kind {
		case registry.MethodNPM:
			if _, err := i.runner().RunContext(ctx, "npm", "uninstall", "-g", m.NpmPackage); err == nil {
				removed = true
			} else {
				errs = append(errs, fmt.Sprintf("npm uninstall %s: %v", m.NpmPackage, err))
			}
		case registry.MethodGit:
			dir := system.Expand(m.GitDir)
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				if err := os.RemoveAll(dir); err == nil {
					removed = true
				} else {
					errs = append(errs, fmt.Sprintf("remove %s: %v", dir, err))
				}
			}
		case registry.MethodBinary:
			dest := filepath.Join(i.BinDir, m.ExeIn)
			if fi, err := os.Stat(dest); err == nil && !fi.IsDir() {
				if err := os.Remove(dest); err == nil {
					removed = true
				} else {
					errs = append(errs, fmt.Sprintf("remove %s: %v", dest, err))
				}
			}
		case registry.MethodScript:
			// Best effort: remove the tool's known install dirs.
			for _, dir := range scriptDirs(t) {
				full := system.Expand(dir)
				if fi, err := os.Stat(full); err == nil && fi.IsDir() {
					if err := os.RemoveAll(full); err == nil {
						removed = true
					} else {
						errs = append(errs, fmt.Sprintf("remove %s: %v", full, err))
					}
				}
			}
		}
	}
	if removed {
		return nil
	}
	if len(errs) == 0 {
		return fmt.Errorf("nothing to remove for %s", t.Name)
	}
	return fmt.Errorf("remove %s failed: %s", t.Name, strings.Join(errs, "; "))
}

// scriptDirs lists the directories the official installers create.
func scriptDirs(t *registry.Tool) []string {
	switch t.Name {
	case "flyctl":
		return []string{"~/.fly"}
	case "sdkman":
		return []string{"~/.sdkman"}
	case "grok":
		return []string{"~/.grok"}
	case "claude", "codex":
		// native installs live under ~/.local/share and ~/.local/bin;
		// removal is best-effort.
		return []string{"~/.local/share/claude-code", "~/.local/share/codex", "~/.local/bin/claude", "~/.local/bin/codex"}
	default:
		return nil
	}
}

// apply executes one install/update method.
func (i *Installer) apply(ctx context.Context, t *registry.Tool, m registry.Method, version string) error {
	switch m.Kind {
	case registry.MethodScript:
		return i.script(ctx, m.Script)
	case registry.MethodNPM:
		return i.npm(ctx, m)
	case registry.MethodGit:
		return i.git(ctx, m)
	case registry.MethodSelf:
		return i.self(ctx, t, m)
	case registry.MethodBinary:
		return i.binary(ctx, t, m, version)
	default:
		return fmt.Errorf("unknown method kind %q", m.Kind)
	}
}

// script runs a bash -c snippet.
func (i *Installer) script(ctx context.Context, script string) error {
	if i.DryRun {
		i.logf("    [dry-run] bash -c %s", script)
		return nil
	}
	if _, err := i.runner().RunBashContext(ctx, script); err != nil {
		return fmt.Errorf("installer script failed: %w", err)
	}
	return nil
}

// npm installs/updates a global npm package.
func (i *Installer) npm(ctx context.Context, m registry.Method) error {
	if _, err := system.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found on PATH: %w", err)
	}
	if i.DryRun {
		i.logf("    [dry-run] npm install -g %s", m.NpmPackage)
		return nil
	}
	args := []string{"install", "-g", m.NpmPackage}
	if _, err := i.runner().RunContext(ctx, "npm", args...); err != nil {
		return fmt.Errorf("npm install -g %s failed: %w", m.NpmPackage, err)
	}
	return nil
}

// git clones (or pulls) a repository.
func (i *Installer) git(ctx context.Context, m registry.Method) error {
	dir := system.Expand(m.GitDir)
	if _, err := system.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH: %w", err)
	}
	if i.DryRun {
		i.logf("    [dry-run] git clone %s %s", m.GitRepo, dir)
		return nil
	}
	if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
		// Already cloned: pull latest.
		cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
		cmd.Env = os.Environ()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git pull %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", m.GitRepo, dir)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("git clone %s: %w: %s", m.GitRepo, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// self runs the tool's own subcommand (e.g. mise self-update) against the
// detected binary path.
func (i *Installer) self(ctx context.Context, t *registry.Tool, m registry.Method) error {
	path, ok := t.DetectPath()
	if !ok {
		return fmt.Errorf("%s not found", t.Name)
	}
	if i.DryRun {
		i.logf("    [dry-run] %s %s", path, strings.Join(m.SelfCmd, " "))
		return nil
	}
	if _, err := i.runner().RunContext(ctx, path, m.SelfCmd...); err != nil {
		return fmt.Errorf("%s %s failed: %w", t.Name, strings.Join(m.SelfCmd, " "), err)
	}
	return nil
}

// binary downloads a GitHub release tarball, extracts the binary and places
// it in BinDir.
func (i *Installer) binary(ctx context.Context, t *registry.Tool, m registry.Method, version string) error {
	osName, arch := registry.OSArch()
	if mapped, ok := m.ArchMap[arch]; ok {
		arch = mapped
	}
	ver := version
	if ver == "" || ver == "latest" {
		latest, err := i.latestVersion(ctx, m.Repo)
		if err != nil {
			return fmt.Errorf("resolve latest %s: %w", t.Name, err)
		}
		ver = latest
	} else {
		ver = registry.VersionForBinary(ver)
	}
	asset := strings.NewReplacer(
		"{version}", ver,
		"{os}", osName,
		"{arch}", arch,
		"{exe}", m.ExeIn,
	).Replace(m.Asset)
	url := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", m.Repo, ver, asset)

	dest := filepath.Join(i.BinDir, m.ExeIn)
	if i.DryRun {
		i.logf("    [dry-run] download %s -> %s", url, dest)
		return nil
	}
	if _, err := os.Stat(dest); err == nil && version != "" && version != "latest" {
		i.logf("    %s already at %s", dest, version)
		return nil
	}

	i.logf("    downloading %s", url)
	tmp, err := os.CreateTemp("", "climan-*"+filepath.Ext(asset))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := system.Download(ctx, url, tmp.Name()); err != nil {
		return err
	}
	if err := extractBinary(tmp.Name(), dest, m.ExeIn); err != nil {
		return err
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return err
	}
	i.logf("    installed %s -> %s", m.ExeIn, dest)
	return nil
}

// latestVersion resolves the newest release tag of a GitHub repo.
func (i *Installer) latestVersion(ctx context.Context, repo string) (string, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "climan")
	resp, err := i.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api %s: %s", api, resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("no releases found for %s", repo)
	}
	return registry.VersionForBinary(rel.TagName), nil
}

// extractBinary pulls ExeIn out of a tar.gz archive into dest.
func extractBinary(archive, dest, want string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if name != want {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("archive %s contains no file named %q", archive, want)
}

// orVersion formats a desired version for log output.
func orVersion(v string) string {
	if v == "" {
		return "latest"
	}
	return v
}
