package registry

import (
	"encoding/json"
	"fmt"
	"sort"
)

type catalogFile struct {
	Schema int        `json:"schema"`
	Tools  []toolJSON `json:"tools"`
}

type toolJSON struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Category    string       `json:"category"`
	Homepage    string       `json:"homepage"`
	Binaries    []string     `json:"binaries"`
	VersionArgs []string     `json:"version_args"`
	Init        *bool        `json:"init"`
	Notes       string       `json:"notes"`
	Language    string       `json:"language"`
	Detect      *detectJSON  `json:"detect"`
	Version     *versionJSON `json:"version"`
	Install     []methodJSON `json:"install"`
	Update      []methodJSON `json:"update"`
}

type detectJSON struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type versionJSON struct {
	Kind   string `json:"kind"`
	Script string `json:"script"`
}

type methodJSON struct {
	Kind       string            `json:"kind"`
	Script     string            `json:"script"`
	Repo       string            `json:"repo"`
	Asset      string            `json:"asset"`
	ExeIn      string            `json:"exe_in"`
	ArchMap    map[string]string `json:"arch_map"`
	NpmPackage string            `json:"npm_package"`
	NpmBin     string            `json:"npm_bin"`
	GitRepo    string            `json:"git_repo"`
	GitDir     string            `json:"git_dir"`
	SelfCmd    []string          `json:"self_cmd"`
	Slug       string            `json:"slug"`
	Note       string            `json:"note"`
}

// ParseCatalog builds a registry from the API /v1/climan/tools JSON document.
func ParseCatalog(data []byte) (*Registry, error) {
	var doc catalogFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse tools catalog: %w", err)
	}
	if doc.Schema != 0 && doc.Schema != 1 {
		return nil, fmt.Errorf("unsupported tools catalog schema %d", doc.Schema)
	}
	r := &Registry{tools: map[string]*Tool{}}
	for _, raw := range doc.Tools {
		t, err := raw.toTool()
		if err != nil {
			return nil, err
		}
		if t.Name == "" {
			return nil, fmt.Errorf("catalog tool missing name")
		}
		r.tools[t.Name] = t
		r.order = append(r.order, t.Name)
	}
	sort.Strings(r.order)
	return r, nil
}

func (raw toolJSON) toTool() (*Tool, error) {
	init := true
	if raw.Init != nil {
		init = *raw.Init
	}
	t := &Tool{
		Name:        raw.Name,
		Description: raw.Description,
		Category:    Category(raw.Category),
		Homepage:    raw.Homepage,
		Binaries:    raw.Binaries,
		VersionArgs: raw.VersionArgs,
		Notes:       raw.Notes,
		Init:        init,
		Language:    raw.Language,
	}
	if raw.Detect != nil {
		t.DetectKind = raw.Detect.Kind
		t.DetectFile = raw.Detect.Path
	}
	if raw.Version != nil {
		t.VersionKind = raw.Version.Kind
		t.VersionScript = raw.Version.Script
	}
	for _, m := range raw.Install {
		parsed, err := m.toMethod()
		if err != nil {
			return nil, fmt.Errorf("%s install: %w", raw.Name, err)
		}
		t.Install = append(t.Install, parsed)
	}
	for _, m := range raw.Update {
		parsed, err := m.toMethod()
		if err != nil {
			return nil, fmt.Errorf("%s update: %w", raw.Name, err)
		}
		t.Update = append(t.Update, parsed)
	}
	return t, nil
}

func (m methodJSON) toMethod() (Method, error) {
	kind := MethodKind(m.Kind)
	switch kind {
	case MethodScript, MethodBinary, MethodNPM, MethodGit, MethodSelf, MethodReleases, MethodSdks:
	case "":
		return Method{}, fmt.Errorf("method missing kind")
	default:
		return Method{}, fmt.Errorf("unknown method kind %q", m.Kind)
	}
	return Method{
		Kind:       kind,
		Script:     m.Script,
		Repo:       m.Repo,
		Asset:      m.Asset,
		ExeIn:      m.ExeIn,
		ArchMap:    m.ArchMap,
		NpmPackage: m.NpmPackage,
		NpmBin:     m.NpmBin,
		GitRepo:    m.GitRepo,
		GitDir:     m.GitDir,
		SelfCmd:    m.SelfCmd,
		Slug:       m.Slug,
		Note:       m.Note,
	}, nil
}
