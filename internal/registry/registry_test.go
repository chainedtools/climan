package registry

import (
	"strings"
	"testing"
)

func TestNewRegistryHasAllTools(t *testing.T) {
	r := New()
	want := []string{"mise", "asdf", "sdkman", "flyctl", "doctl", "grok", "codex", "claude"}
	for _, name := range want {
		if _, ok := r.Get(name); !ok {
			t.Errorf("registry missing %q", name)
		}
	}
	if got := len(r.All()); got != len(want) {
		t.Errorf("All() returned %d tools, want %d", got, len(want))
	}
}

func TestNamesSorted(t *testing.T) {
	r := New()
	names := r.Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("names not sorted: %v", names)
		}
	}
}

func TestUnknownToolRejected(t *testing.T) {
	r := New()
	if _, ok := r.Get("definitely-not-a-tool"); ok {
		t.Fatal("expected unknown tool to be rejected")
	}
}

func TestCategories(t *testing.T) {
	r := New()
	seen := map[Category]int{}
	for _, t := range r.All() {
		seen[t.Category]++
	}
	for _, c := range []Category{CategoryVersionManager, CategoryCloud, CategoryAI} {
		if seen[c] == 0 {
			t.Errorf("category %s has no tools", c)
		}
	}
}

func TestEveryToolHasInstallMethod(t *testing.T) {
	r := New()
	for _, tool := range r.All() {
		if len(tool.Install) == 0 {
			t.Errorf("%s has no install methods", tool.Name)
		}
		for _, m := range tool.Install {
			switch m.Kind {
			case MethodScript:
				if !strings.Contains(m.Script, "curl") && !strings.Contains(m.Script, "wget") {
					t.Errorf("%s script method has no downloader: %q", tool.Name, m.Script)
				}
			case MethodBinary:
				if m.Repo == "" || m.Asset == "" || m.ExeIn == "" {
					t.Errorf("%s binary method incomplete: %+v", tool.Name, m)
				}
			case MethodNPM:
				if m.NpmPackage == "" {
					t.Errorf("%s npm method missing package", tool.Name)
				}
			case MethodGit:
				if m.GitRepo == "" || m.GitDir == "" {
					t.Errorf("%s git method incomplete: %+v", tool.Name, m)
				}
			}
		}
	}
}

func TestUpdateMethodsFallBackToInstall(t *testing.T) {
	r := New()
	for _, tool := range r.All() {
		got := tool.UpdateMethods()
		if len(got) == 0 {
			t.Errorf("%s has no update strategy", tool.Name)
		}
	}
}

func TestVersionForBinary(t *testing.T) {
	cases := map[string]string{
		"v1.2.3": "1.2.3",
		"1.2.3":  "1.2.3",
		" v2.0 ": "2.0",
	}
	for in, want := range cases {
		if got := VersionForBinary(in); got != want {
			t.Errorf("VersionForBinary(%q) = %q, want %q", in, got, want)
		}
	}
}
