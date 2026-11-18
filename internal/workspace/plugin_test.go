package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	repoRoot    = "../.."
	pluginEntry = "plugins/ponchimap/skills/ponchimap/SKILL.md"
)

func TestPluginEntryNamesEveryPlacedSkill(t *testing.T) {
	entry, err := os.ReadFile(filepath.Join(repoRoot, pluginEntry))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range Agents {
		placed := "`" + a.Folder + "/SKILL.md`"
		if !strings.Contains(string(entry), placed) {
			t.Errorf("the plugin's entry Skill does not name %s, where init places the Skill for %s", placed, a.Name)
		}
	}
}

func TestMarketplacesListThePlugin(t *testing.T) {
	var claude struct {
		Plugins []struct {
			Name   string
			Source string
		}
	}
	var codex struct {
		Plugins []struct {
			Name   string
			Source struct{ Path string }
		}
	}
	readJSON(t, ".claude-plugin/marketplace.json", &claude)
	readJSON(t, ".agents/plugins/marketplace.json", &codex)
	if len(claude.Plugins) != 1 || len(codex.Plugins) != 1 {
		t.Fatalf("each marketplace lists one plugin; Claude Code lists %d, Codex %d", len(claude.Plugins), len(codex.Plugins))
	}
	listed := []struct{ marketplace, name, path string }{
		{"Claude Code", claude.Plugins[0].Name, claude.Plugins[0].Source},
		{"Codex", codex.Plugins[0].Name, codex.Plugins[0].Source.Path},
	}
	for _, p := range listed {
		if p.name != "ponchimap" {
			t.Errorf("the %s marketplace names the plugin %q; people install it as ponchimap@ponchimap", p.marketplace, p.name)
		}
		if got, want := filepath.Join(p.path, "skills/ponchimap/SKILL.md"), filepath.Clean(pluginEntry); got != want {
			t.Errorf("the %s marketplace points at %s, which does not hold %s", p.marketplace, p.path, want)
		}
	}
}

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
