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

package vault

import (
	"fmt"
	"os"
)

const (
	defaultServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// authenticate authenticates with Vault using the configured method
func (v *VaultBackend) authenticate(config *Config) error {
	switch config.AuthMethod {
	case "kubernetes":
		return v.authenticateKubernetes(config)
	case "token":
		return v.authenticateToken(config)
	case "approle":
		return fmt.Errorf("approle authentication not yet implemented")
	default:
		return fmt.Errorf("unsupported auth method: %s", config.AuthMethod)
	}
}

// authenticateKubernetes authenticates using Kubernetes service account
func (v *VaultBackend) authenticateKubernetes(config *Config) error {
	// Read service account token
	tokenBytes, err := os.ReadFile(defaultServiceAccountTokenPath)
	if err != nil {
		return fmt.Errorf("failed to read service account token: %w", err)
	}
	jwt := string(tokenBytes)

	// Prepare login data
	loginData := map[string]interface{}{
		"jwt":  jwt,
		"role": config.KubernetesAuthRole,
	}

	// Login to Vault
	authPath := fmt.Sprintf("auth/%s/login", config.KubernetesAuthPath)
	secret, err := v.client.Logical().Write(authPath, loginData)
	if err != nil {
		return fmt.Errorf("kubernetes auth login failed: %w", err)
	}

	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return fmt.Errorf("kubernetes auth returned no token")
	}

	// Set the token
	v.client.SetToken(secret.Auth.ClientToken)

	return nil
}

// authenticateToken authenticates using a pre-configured token
func (v *VaultBackend) authenticateToken(config *Config) error {
	if config.Token == "" {
		return fmt.Errorf("token is required for token authentication")
	}

	v.client.SetToken(config.Token)

	// Verify token is valid
	_, err := v.client.Auth().Token().LookupSelf()
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	return nil
}
