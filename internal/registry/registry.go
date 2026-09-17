// Package registry is the declarative catalog of CLIs that climan knows how
// to install, update and remove. Each tool carries its own install/update
// methods, detection rules and version commands, so the rest of climan can
// treat every tool uniformly.
package registry

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"go.solved.gg/climan/internal/system"
)

// Category groups tools in the TUI and in list output.
type Category string

const (
	CategoryVersionManager Category = "version-manager"
	CategoryCloud          Category = "cloud"
	CategoryAI             Category = "ai"
	CategoryChained        Category = "chained"
	CategorySDK            Category = "sdk"
)

func (c Category) Label() string {
	switch c {
	case CategoryVersionManager:
		return "Version Managers"
	case CategoryCloud:
		return "Cloud"
	case CategoryAI:
		return "AI Coding"
	case CategoryChained:
		return "chained.tools"
	case CategorySDK:
		return "SDKs"
	default:
		return string(c)
	}
}

// MethodKind is the installation strategy used for a tool.
type MethodKind string

const (
	// MethodScript runs a curl | bash style official installer.
	MethodScript MethodKind = "script"
	// MethodBinary downloads a release tarball from GitHub Releases.
	MethodBinary MethodKind = "binary"
	// MethodNPM installs a global npm package.
	MethodNPM MethodKind = "npm"
	// MethodGit clones a git repository (used by asdf).
	MethodGit MethodKind = "git"
	// MethodSelf runs a subcommand provided by the tool itself (e.g. `mise self-update`).
	MethodSelf MethodKind = "self"
	// MethodReleases downloads a first-party artifact from the locked releases host.
	MethodReleases MethodKind = "releases"
	// MethodSdks downloads a public SDK tarball from sdks.chained.tools.
	MethodSdks MethodKind = "sdks"
)

// Method describes a single way to install/update a tool.
type Method struct {
	Kind MethodKind

	// Script: full bash snippet, e.g. `curl -fsSL https://… | bash`.
	Script string

	// Binary: GitHub repo in "owner/name" form and an asset-name template.
	// Placeholders: {version} (no leading v), {os}, {arch}, {exe}.
	Repo    string
	Asset   string
	ExeIn   string // name of the binary inside the archive
	ArchMap map[string]string

	// NPM: package name and the command it provides.
	NpmPackage string
	NpmBin     string

	// Git: repository URL and destination directory (expanded from ~).
	GitRepo string
	GitDir  string

	// Self: args run against the installed binary, e.g. ["self-update"].
	SelfCmd []string

	// Releases / SDKs: product slug on the corresponding host.
	Slug string

	// Note is surfaced to the user after running this method.
	Note string
}

// Tool is the declarative description of one CLI.
type Tool struct {
	Name        string
	Description string
	Category    Category
	Homepage    string

	// Binaries lists command names used to detect the tool on PATH,
	// in priority order.
	Binaries []string

	// VersionArgs are appended to the detected binary to print a version,
	// e.g. ["--version"].
	VersionArgs []string

	// Install methods are tried in order until one succeeds.
	Install []Method
	// Update methods are tried in order until one succeeds. When empty,
	// Install is used as the update strategy.
	Update []Method

	// Custom detection override (asdf/sdkman live outside PATH).
	Detect func() (string, bool)
	// Custom version override (sdkman needs shell sourcing).
	Version func(path string) (string, error)
	// Notes shown in `climan doctor` / after install.
	Notes string

	// Init is true when `climan init` should declare this tool.
	Init bool
	// Language is set for SDK recipes (zig/gleam/elixir).
	Language string
	// DetectKind is "path" (default) or "file".
	DetectKind string
	// DetectFile is a ~/… path used when DetectKind is "file".
	DetectFile string
	// VersionKind is "args" (default) or "bash".
	VersionKind string
	// VersionScript is a bash snippet used when VersionKind is "bash".
	VersionScript string
}

// Registry holds every known tool, keyed by name.
type Registry struct {
	tools map[string]*Tool
	order []string
}

// New builds the default registry with all supported tools.
func New() *Registry {
	r := &Registry{tools: map[string]*Tool{}}
	for _, t := range allTools() {
		t.Init = true
		r.tools[t.Name] = t
		r.order = append(r.order, t.Name)
	}
	sort.Strings(r.order)
	return r
}

// Get returns the tool with the given name.
func (r *Registry) Get(name string) (*Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Names returns all tool names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// All returns every tool, sorted by name.
func (r *Registry) All() []*Tool {
	out := make([]*Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}
	return out
}

// ByCategory returns tools grouped by category (category order preserved,
// names sorted within).
func (r *Registry) ByCategory() [][2]any {
	groups := map[Category][]*Tool{}
	for _, t := range r.All() {
		groups[t.Category] = append(groups[t.Category], t)
	}
	var order []Category
	for _, c := range []Category{CategoryVersionManager, CategoryCloud, CategoryAI, CategoryChained, CategorySDK} {
		if _, ok := groups[c]; ok {
			order = append(order, c)
		}
	}
	out := make([][2]any, 0, len(order))
	for _, c := range order {
		out = append(out, [2]any{c, groups[c]})
	}
	return out
}

// SuggestNames returns a human-friendly list of valid names for error messages.
func (r *Registry) SuggestNames() string {
	return strings.Join(r.Names(), ", ")
}

// allTools returns the built-in catalog. Data sourced from each project's
// official documentation (see README.md for per-tool references).
func allTools() []*Tool {
	return []*Tool{
		miseTool(),
		asdfTool(),
		sdkmanTool(),
		flyctlTool(),
		doctlTool(),
		grokTool(),
		codexTool(),
		claudeTool(),
	}
}

// ---- tools ----

func miseTool() *Tool {
	return &Tool{
		Name:        "mise",
		Description: "Polyglot runtime & env version manager (successor to asdf/rtx)",
		Category:    CategoryVersionManager,
		Homepage:    "https://mise.jdx.dev",
		Binaries:    []string{"mise"},
		VersionArgs: []string{"--version"},
		Install: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://mise.run | sh`,
				Note: "binary installed to ~/.local/bin/mise"},
		},
		Update: []Method{
			{Kind: MethodSelf, SelfCmd: []string{"self-update"}, Note: "mise self-update"},
			{Kind: MethodScript, Script: `curl -fsSL https://mise.run | sh`},
		},
		Notes: "Activate with: eval \"$(mise activate bash)\" (or zsh/fish)",
	}
}

func asdfTool() *Tool {
	return &Tool{
		Name:        "asdf",
		Description: "Extendable version manager with a plugin ecosystem",
		Category:    CategoryVersionManager,
		Homepage:    "https://asdf-vm.com",
		Binaries:    []string{"asdf"},
		VersionArgs: []string{"--version"},
		Install: []Method{
			{Kind: MethodGit, GitRepo: "https://github.com/asdf-vm/asdf.git", GitDir: "~/.asdf",
				Note: "cloned to ~/.asdf"},
		},
		Update: []Method{
			{Kind: MethodGit, GitRepo: "https://github.com/asdf-vm/asdf.git", GitDir: "~/.asdf"},
		},
		Detect: func() (string, bool) {
			if p := homeFile(".asdf/bin/asdf"); p != "" {
				return p, true
			}
			return "", false
		},
		Notes: "Add to your shell: source ~/.asdf/asdf.sh",
	}
}

func sdkmanTool() *Tool {
	return &Tool{
		Name:        "sdkman",
		Description: "SDK manager for JVM tooling (Java, Kotlin, Gradle, Maven…)",
		Category:    CategoryVersionManager,
		Homepage:    "https://sdkman.io",
		Binaries:    []string{"sdk"},
		VersionArgs: []string{"version"},
		Install: []Method{
			// ci=true + rcupdate=false keeps the installer non-interactive
			// and avoids silently editing shell rc files.
			{Kind: MethodScript, Script: `curl -s "https://get.sdkman.io?ci=true&rcupdate=false" | bash`,
				Note: "installed to ~/.sdkman (rc files untouched)"},
		},
		Update: []Method{
			{Kind: MethodScript, Script: `bash -c 'source "$HOME/.sdkman/bin/sdkman-init.sh" && sdk selfupdate'`},
		},
		Detect: func() (string, bool) {
			if homeFile(".sdkman/bin/sdkman-init.sh") != "" {
				return "~/.sdkman/bin/sdkman-init.sh", true
			}
			return "", false
		},
		Version: func(path string) (string, error) {
			return runBash(`source "$HOME/.sdkman/bin/sdkman-init.sh" && sdk version`)
		},
		Notes: "Add to your shell: source \"$HOME/.sdkman/bin/sdkman-init.sh\"",
	}
}

func flyctlTool() *Tool {
	return &Tool{
		Name:        "flyctl",
		Description: "Fly.io deployment platform CLI (fly)",
		Category:    CategoryCloud,
		Homepage:    "https://fly.io/docs/flyctl",
		Binaries:    []string{"flyctl", "fly"},
		VersionArgs: []string{"version"},
		Install: []Method{
			{Kind: MethodScript, Script: `curl -L https://fly.io/install.sh | sh`,
				Note: "installed to ~/.fly/bin/flyctl"},
		},
		Update: []Method{
			{Kind: MethodScript, Script: `curl -L https://fly.io/install.sh | sh`},
		},
		Notes: "Binary lives in ~/.fly/bin; add it to PATH if not present",
	}
}

func doctlTool() *Tool {
	return &Tool{
		Name:        "doctl",
		Description: "DigitalOcean API CLI",
		Category:    CategoryCloud,
		Homepage:    "https://docs.digitalocean.com/reference/doctl",
		Binaries:    []string{"doctl"},
		VersionArgs: []string{"version"},
		Install: []Method{
			{Kind: MethodBinary, Repo: "digitalocean/doctl",
				Asset: "doctl-{version}-{os}-{arch}.tar.gz", ExeIn: "doctl",
				ArchMap: map[string]string{"amd64": "amd64", "arm64": "arm64"}},
		},
		Update: []Method{
			{Kind: MethodBinary, Repo: "digitalocean/doctl",
				Asset: "doctl-{version}-{os}-{arch}.tar.gz", ExeIn: "doctl",
				ArchMap: map[string]string{"amd64": "amd64", "arm64": "arm64"}},
		},
	}
}

func grokTool() *Tool {
	return &Tool{
		Name:        "grok",
		Description: "xAI Grok CLI — coding agent in the terminal",
		Category:    CategoryAI,
		Homepage:    "https://x.ai/cli",
		Binaries:    []string{"grok"},
		VersionArgs: []string{"--version"},
		Install: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://x.ai/cli/install.sh | bash`,
				Note: "official installer; sign in with `grok login`"},
		},
		Update: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://x.ai/cli/install.sh | bash`},
		},
	}
}

func codexTool() *Tool {
	return &Tool{
		Name:        "codex",
		Description: "OpenAI Codex CLI — coding agent",
		Category:    CategoryAI,
		Homepage:    "https://developers.openai.com/codex",
		Binaries:    []string{"codex"},
		VersionArgs: []string{"--version"},
		Install: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://chatgpt.com/codex/install.sh | sh`,
				Note: "official standalone installer"},
			{Kind: MethodNPM, NpmPackage: "@openai/codex", NpmBin: "codex"},
		},
		Update: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://chatgpt.com/codex/install.sh | sh`},
			{Kind: MethodNPM, NpmPackage: "@openai/codex", NpmBin: "codex"},
		},
	}
}

func claudeTool() *Tool {
	return &Tool{
		Name:        "claude",
		Description: "Anthropic Claude Code — coding agent",
		Category:    CategoryAI,
		Homepage:    "https://code.claude.com",
		Binaries:    []string{"claude"},
		VersionArgs: []string{"--version"},
		Install: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://claude.ai/install.sh | bash`,
				Note: "native install auto-updates in the background"},
			{Kind: MethodNPM, NpmPackage: "@anthropic-ai/claude-code", NpmBin: "claude",
				Note: "npm fallback (requires Node.js 22+)"},
		},
		Update: []Method{
			{Kind: MethodScript, Script: `curl -fsSL https://claude.ai/install.sh | bash`},
			{Kind: MethodNPM, NpmPackage: "@anthropic-ai/claude-code@latest", NpmBin: "claude"},
		},
	}
}

// DetectPath finds the tool on the system, honouring a custom Detect override.
// Returns the resolved path (or a human-readable location) and whether it
// was found.
func (t *Tool) DetectPath() (string, bool) {
	if t.Detect != nil {
		return t.Detect()
	}
	if t.DetectKind == "file" && t.DetectFile != "" {
		p := system.Expand(t.DetectFile)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		return "", false
	}
	for _, bin := range t.Binaries {
		if p, err := lookPath(bin); err == nil {
			return p, true
		}
	}
	return "", false
}

// VersionOf runs the tool's version command against the given path.
func (t *Tool) VersionOf(path string) (string, error) {
	if t.Version != nil {
		return t.Version(path)
	}
	if t.VersionKind == "bash" && t.VersionScript != "" {
		return runBash(t.VersionScript)
	}
	if len(t.Binaries) == 0 {
		return "", fmt.Errorf("no binaries defined for %s", t.Name)
	}
	return runCmd(path, t.VersionArgs...)
}

// PrimaryBinary returns the first binary name (used for removal paths).
func (t *Tool) PrimaryBinary() string {
	if len(t.Binaries) == 0 {
		return t.Name
	}
	return t.Binaries[0]
}

// UpdateMethods returns the update strategy, falling back to Install.
func (t *Tool) UpdateMethods() []Method {
	if len(t.Update) > 0 {
		return t.Update
	}
	return t.Install
}

// VersionForBinary returns the tool's version number given a GitHub release
// tag (strips a leading "v"). Used by the binary installer.
func VersionForBinary(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

// OSArch returns the lowercase os/arch pair used in asset templates.
func OSArch() (os, arch string) {
	os = runtime.GOOS
	switch runtime.GOOS {
	case "darwin":
		os = "darwin"
	case "linux":
		os = "linux"
	}
	arch = runtime.GOARCH
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	case "386":
		arch = "386"
	}
	return os, arch
}
