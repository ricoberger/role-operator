package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitLoadsPresetsFromFile(t *testing.T) {
	t.Cleanup(Reset)

	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	content := `presets:
  - name: readonly
    namespaces:
      - team-a
    roleRules:
      - apiGroups: [""]
        resources: ["configmaps"]
        verbs: ["get", "list", "watch"]
    clusterRoleRules:
      - apiGroups: [""]
        resources: ["namespaces"]
        verbs: ["get", "list", "watch"]
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("ROLE_OPERATOR_CONFIG", file)

	if err := Init(); err != nil {
		t.Fatalf("Init returned an error: %v", err)
	}

	preset := GetPreset("readonly")
	if preset == nil {
		t.Fatal("expected preset \"readonly\" to be loaded, got nil")
	}
	if len(preset.Namespaces) != 1 || preset.Namespaces[0] != "team-a" {
		t.Errorf("unexpected namespaces: %v", preset.Namespaces)
	}
	if len(preset.RoleRules) != 1 || preset.RoleRules[0].Resources[0] != "configmaps" {
		t.Errorf("unexpected roleRules: %v", preset.RoleRules)
	}
	if len(preset.ClusterRoleRules) != 1 || preset.ClusterRoleRules[0].Resources[0] != "namespaces" {
		t.Errorf("unexpected clusterRoleRules: %v", preset.ClusterRoleRules)
	}
}

func TestInitWithoutConfigFile(t *testing.T) {
	t.Cleanup(Reset)
	Reset()

	t.Setenv("ROLE_OPERATOR_CONFIG", "")

	if err := Init(); err != nil {
		t.Fatalf("Init returned an error: %v", err)
	}
	if p := GetPreset("anything"); p != nil {
		t.Errorf("expected no preset without a config file, got %v", p)
	}
}

func TestGetPresetNotFound(t *testing.T) {
	t.Cleanup(Reset)

	SetPresets([]Preset{{Name: "existing"}})

	if p := GetPreset("missing"); p != nil {
		t.Errorf("expected nil for missing preset, got %v", p)
	}
	if p := GetPreset("existing"); p == nil {
		t.Error("expected to find preset \"existing\", got nil")
	}
}
