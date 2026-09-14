package installer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.solved.gg/climan/internal/registry"
	"go.solved.gg/climan/internal/system"
)

// makeTarball builds a tar.gz containing the given files.
func makeTarball(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, "bundle.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()
	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestExtractBinary(t *testing.T) {
	dir := t.TempDir()
	arc := makeTarball(t, dir, map[string]string{
		"doctl-1.2.3-linux-amd64/doctl": "#!/bin/sh\necho doctl\n",
		"README.md":                     "ignore me",
	})
	dest := filepath.Join(dir, "out", "doctl")
	if err := extractBinary(arc, dest, "doctl"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/sh\necho doctl\n" {
		t.Errorf("extracted content mismatch: %q", data)
	}
}

func TestExtractBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	arc := makeTarball(t, dir, map[string]string{"other": "x"})
	err := extractBinary(arc, filepath.Join(dir, "doctl"), "doctl")
	if err == nil {
		t.Fatal("expected error when binary is absent from archive")
	}
}

func TestStatusMissing(t *testing.T) {
	i := New()
	reg := registry.New()
	tool, _ := reg.Get("mise")
	// Force a name that cannot exist: use a synthetic tool with a bogus binary.
	synth := &registry.Tool{Name: "nonexistent-mise", Binaries: []string{"climan-does-not-exist-xyz"}, VersionArgs: []string{"--version"}}
	st := i.Status(synth, "latest")
	if st.State != StateMissing {
		t.Errorf("state = %s, want missing", st.State)
	}
	if tool == nil {
		t.Fatal("mise should exist in registry")
	}
}

func TestStatusInstalled(t *testing.T) {
	i := New()
	// Use a known, always-present binary: "sh".
	synth := &registry.Tool{
		Name:        "sh-tool",
		Binaries:    []string{"sh"},
		VersionArgs: []string{"-c", "echo 1.0"},
	}
	st := i.Status(synth, "latest")
	if st.State == StateMissing {
		t.Fatal("sh should be detected")
	}
	if st.Path == "" {
		t.Error("path should be set")
	}
}

func TestDryRunInstallDoesNotTouchSystem(t *testing.T) {
	i := New()
	i.DryRun = true
	tool := &registry.Tool{
		Name:     "fake",
		Binaries: []string{"climan-fake-xyz"},
		Install:  []registry.Method{{Kind: registry.MethodScript, Script: `curl -fsSL https://example.invalid | bash`}},
	}
	if err := i.Install(context.Background(), tool, "latest"); err != nil {
		t.Fatalf("dry-run install should succeed: %v", err)
	}
}

func TestVersionForBinaryInInstaller(t *testing.T) {
	// latestVersion needs network; exercise the URL builder instead via a
	// synthetic binary method dry run.
	i := New()
	i.DryRun = true
	tool := &registry.Tool{
		Name:     "doctl",
		Binaries: []string{"doctl"},
		Install: []registry.Method{{
			Kind: registry.MethodBinary, Repo: "digitalocean/doctl",
			Asset: "doctl-{version}-{os}-{arch}.tar.gz", ExeIn: "doctl",
			ArchMap: map[string]string{"amd64": "amd64", "arm64": "arm64"},
		}},
	}
	if err := i.Install(context.Background(), tool, "1.99.0"); err != nil {
		t.Fatalf("dry-run binary install should succeed: %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home := system.Home()
	if got := system.Expand("~/x"); got != filepath.Join(home, "x") {
		t.Errorf("Expand(~) = %s", got)
	}
}
