package config

import (
	"os"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

type Config struct {
	Presets []Preset `json:"presets,omitempty"`
}

type Preset struct {
	Name             string              `json:"name,omitempty"`
	Namespaces       []string            `json:"namespaces,omitempty"`
	RoleRules        []rbacv1.PolicyRule `json:"roleRules,omitempty"`
	ClusterRoleRules []rbacv1.PolicyRule `json:"clusterRoleRules,omitempty"`
}

type Client interface {
	GetPreset(preset string) *Preset
}

type client struct {
	config *Config
}

func NewClient() (Client, error) {
	var config *Config

	if configFile := os.Getenv("ROLE_OPERATOR_CONFIG"); configFile != "" {
		//nolint:gosec
		configContent, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(configContent, &config)
		if err != nil {
			return nil, err
		}

		log.Log.Info("loaded configuration", "file", configFile, "presets", len(config.Presets))
	}

	return &client{
		config: config,
	}, nil
}

func (c *client) GetPreset(preset string) *Preset {
	if c.config == nil || len(c.config.Presets) == 0 {
		return nil
	}

	for _, p := range c.config.Presets {
		if p.Name == preset {
			return &p
		}
	}

	return nil
}
