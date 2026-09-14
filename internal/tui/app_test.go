package tui

import (
	"strings"
	"testing"

	"go.solved.gg/climan/internal/config"
	"go.solved.gg/climan/internal/installer"
)

func newTestModel() *Model {
	inst := installer.New()
	inst.Out = nil
	cfg := config.DefaultManifest()
	m := New(inst, cfg)
	m.width, m.height = 120, 40
	return m
}

func TestModelInitializesWithRows(t *testing.T) {
	m := newTestModel()
	if len(m.rows) == 0 {
		t.Fatal("expected status rows after construction")
	}
	names := map[string]bool{}
	for _, r := range m.rows {
		names[r.tool.Name] = true
	}
	for _, want := range []string{"mise", "codex", "claude", "doctl"} {
		if !names[want] {
			t.Errorf("row missing tool %q", want)
		}
	}
}

func TestViewRendersToolsAndFooter(t *testing.T) {
	m := newTestModel()
	v := m.View()
	if !strings.Contains(v.Content, "climan") {
		t.Error("header missing")
	}
	if !strings.Contains(v.Content, "install") || !strings.Contains(v.Content, "quit") {
		t.Error("footer keybindings missing")
	}
	for _, name := range []string{"mise", "flyctl", "grok"} {
		if !strings.Contains(v.Content, name) {
			t.Errorf("view missing tool %q", name)
		}
	}
}

func TestFilterNarrowsRows(t *testing.T) {
	m := newTestModel()
	m.filter.SetValue("ai")
	m.refresh()
	if len(m.rows) != 3 {
		t.Fatalf("filter 'ai' matched %d rows, want 3 (grok, codex, claude)", len(m.rows))
	}
	m.filter.SetValue("zzz-none")
	m.refresh()
	if len(m.rows) != 0 {
		t.Fatalf("bogus filter matched %d rows, want 0", len(m.rows))
	}
}

func TestToggleCatalogShowsAll(t *testing.T) {
	m := newTestModel()
	if len(m.mfst) == len(m.all) {
		t.Skip("manifest already covers full catalog")
	}
	m.showAll = true
	m.refresh()
	if len(m.rows) != len(m.all) {
		t.Errorf("showAll rows = %d, want %d", len(m.rows), len(m.all))
	}
}

func TestCursorClampedOnRefresh(t *testing.T) {
	m := newTestModel()
	m.cursor = 9999
	m.refresh()
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 after refresh", m.cursor)
	}
}

func TestLogBuf(t *testing.T) {
	b := newLogBuf(3)
	b.Add("one")
	b.Add("two\nthree")
	b.Add("four")
	got := b.Snapshot()
	if len(got) != 3 {
		t.Fatalf("log lines = %d, want 3", len(got))
	}
	if got[2] != "four" {
		t.Errorf("last line = %q, want four", got[2])
	}
}
