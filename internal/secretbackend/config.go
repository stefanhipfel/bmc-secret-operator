/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secretbackend

import (
	"fmt"
	"os"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
)

const (
	defaultBackendType = "vault"
)

// Config holds the backend configuration
type Config struct {
	Backend        string
	VaultConfig    *VaultConfigInternal
	OpenBaoConfig  *OpenBaoConfigInternal
	PathTemplate   string
	RegionLabelKey string
	SyncLabel      string
}

// VaultConfigInternal holds internal Vault configuration
type VaultConfigInternal struct {
	Address            string
	AuthMethod         string
	KubernetesAuthRole string
	KubernetesAuthPath string
	Token              string
	MountPath          string
	SkipVerify         bool
	CACert             string
}

// OpenBaoConfigInternal holds internal OpenBao configuration
type OpenBaoConfigInternal struct {
	Address    string
	AuthMethod string
}

// LoadConfigFromCRD converts CRD config to internal config
func LoadConfigFromCRD(crdConfig *configv1alpha1.SecretBackendConfig) (*Config, error) {
	if crdConfig == nil {
		return nil, fmt.Errorf("SecretBackendConfig is nil")
	}

	config := &Config{
		Backend:        crdConfig.Spec.Backend,
		PathTemplate:   crdConfig.Spec.PathTemplate,
		RegionLabelKey: crdConfig.Spec.RegionLabelKey,
		SyncLabel:      crdConfig.Spec.SyncLabel,
	}

	// Set defaults
	if config.PathTemplate == "" {
		config.PathTemplate = "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
	}
	if config.RegionLabelKey == "" {
		config.RegionLabelKey = "region"
	}

	// Load Vault config
	if crdConfig.Spec.VaultConfig != nil {
		vaultCfg := crdConfig.Spec.VaultConfig
		config.VaultConfig = &VaultConfigInternal{
			Address:    vaultCfg.Address,
			AuthMethod: vaultCfg.AuthMethod,
			MountPath:  vaultCfg.MountPath,
		}

		if config.VaultConfig.AuthMethod == "" {
			config.VaultConfig.AuthMethod = "kubernetes"
		}
		if config.VaultConfig.MountPath == "" {
			config.VaultConfig.MountPath = "secret"
		}

		if vaultCfg.KubernetesAuth != nil {
			config.VaultConfig.KubernetesAuthRole = vaultCfg.KubernetesAuth.Role
			config.VaultConfig.KubernetesAuthPath = vaultCfg.KubernetesAuth.Path
			if config.VaultConfig.KubernetesAuthPath == "" {
				config.VaultConfig.KubernetesAuthPath = "kubernetes"
			}
		}

		if vaultCfg.TLSConfig != nil {
			config.VaultConfig.SkipVerify = vaultCfg.TLSConfig.SkipVerify
			config.VaultConfig.CACert = vaultCfg.TLSConfig.CACert
		}
	}

	// Load OpenBao config
	if crdConfig.Spec.OpenBaoConfig != nil {
		config.OpenBaoConfig = &OpenBaoConfigInternal{
			Address:    crdConfig.Spec.OpenBaoConfig.Address,
			AuthMethod: crdConfig.Spec.OpenBaoConfig.AuthMethod,
		}
	}

	return config, nil
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() (*Config, error) {
	backend := os.Getenv("SECRET_BACKEND_TYPE")
	if backend == "" {
		backend = defaultBackendType
	}

	config := &Config{
		Backend:        backend,
		PathTemplate:   getEnvOrDefault("PATH_TEMPLATE", "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"),
		RegionLabelKey: getEnvOrDefault("REGION_LABEL_KEY", "region"),
		SyncLabel:      os.Getenv("SYNC_LABEL"),
	}

	switch backend {
	case defaultBackendType:
		config.VaultConfig = &VaultConfigInternal{
			Address:            os.Getenv("VAULT_ADDR"),
			AuthMethod:         getEnvOrDefault("VAULT_AUTH_METHOD", "kubernetes"),
			KubernetesAuthRole: os.Getenv("VAULT_ROLE"),
			KubernetesAuthPath: getEnvOrDefault("VAULT_KUBERNETES_PATH", "kubernetes"),
			Token:              os.Getenv("VAULT_TOKEN"),
			MountPath:          getEnvOrDefault("VAULT_MOUNT_PATH", "secret"),
			SkipVerify:         os.Getenv("VAULT_SKIP_VERIFY") == "true",
		}

		if config.VaultConfig.Address == "" {
			return nil, fmt.Errorf("VAULT_ADDR environment variable is required")
		}

	case "openbao":
		return nil, fmt.Errorf("OpenBao backend not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported backend type: %s", backend)
	}

	return config, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
