package config

import (
	"os"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

var config *Config

type Config struct {
	Presets []Preset `json:"presets,omitempty"`
}

type Preset struct {
	Name             string              `json:"name,omitempty"`
	Namespaces       []string            `json:"namespaces,omitempty"`
	RoleRules        []rbacv1.PolicyRule `json:"roleRules,omitempty"`
	ClusterRoleRules []rbacv1.PolicyRule `json:"clusterRoleRules,omitempty"`
}

func Init() error {
	if configFile := os.Getenv("ROLE_OPERATOR_CONFIG"); configFile != "" {
		//nolint:gosec
		configContent, err := os.ReadFile(configFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(configContent, &config)
		if err != nil {
			return err
		}

		log.Log.Info("loaded configuration", "file", configFile, "presets", len(config.Presets))
	}

	return nil
}

func GetPreset(preset string) *Preset {
	if config == nil || len(config.Presets) == 0 {
		return nil
	}

	for _, p := range config.Presets {
		if p.Name == preset {
			return &p
		}
	}

	return nil
}

// SetPresets replaces the loaded configuration with the given presets. It is
// intended for use in tests to inject presets without loading a file.
func SetPresets(presets []Preset) {
	config = &Config{Presets: presets}
}

// Reset clears the loaded configuration. It is intended for use in tests.
func Reset() {
	config = nil
}
