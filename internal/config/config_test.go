package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewClientLoadsPresetsFromFile(t *testing.T) {
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

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}

	preset := c.GetPreset("readonly")
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

func TestNewClientWithoutConfigFile(t *testing.T) {
	t.Setenv("ROLE_OPERATOR_CONFIG", "")

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}
	if p := c.GetPreset("anything"); p != nil {
		t.Errorf("expected no preset without a config file, got %v", p)
	}
}

func TestGetPresetNotFound(t *testing.T) {
	c := &client{config: &Config{Presets: []Preset{{Name: "existing"}}}}

	if p := c.GetPreset("missing"); p != nil {
		t.Errorf("expected nil for missing preset, got %v", p)
	}
	if p := c.GetPreset("existing"); p == nil {
		t.Error("expected to find preset \"existing\", got nil")
	}
}

func TestNewClientWithMissingConfigFile(t *testing.T) {
	t.Setenv("ROLE_OPERATOR_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))

	if _, err := NewClient(); err == nil {
		t.Fatal("expected an error for a missing config file, got nil")
	}
}
