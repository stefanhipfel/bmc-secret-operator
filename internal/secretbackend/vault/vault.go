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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
)

// Config holds Vault configuration
type Config struct {
	Address            string
	AuthMethod         string
	KubernetesAuthRole string
	KubernetesAuthPath string
	Token              string
	MountPath          string
	SkipVerify         bool
	CACert             string
}

// VaultBackend implements the Backend interface for HashiCorp Vault
type VaultBackend struct {
	client           *vaultapi.Client
	mountPath        string
	isKVv2           bool
	metricsCollector MetricsCollector
}

// MetricsCollector defines the interface for recording metrics
type MetricsCollector interface {
	RecordAuth(method, backendType string, duration time.Duration, err error)
}

// NewVaultBackend creates a new Vault backend
func NewVaultBackend(config *Config, metricsCollector MetricsCollector) (*VaultBackend, error) {
	// Create Vault client config
	vaultConfig := vaultapi.DefaultConfig()
	vaultConfig.Address = config.Address

	// Configure TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipVerify,
	}

	if config.CACert != "" {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(config.CACert)) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	vaultConfig.HttpClient.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	// Create client
	client, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	backend := &VaultBackend{
		client:           client,
		mountPath:        config.MountPath,
		isKVv2:           true, // Default to KV v2
		metricsCollector: metricsCollector,
	}

	// Authenticate
	if err := backend.authenticate(config); err != nil {
		return nil, fmt.Errorf("failed to authenticate with vault: %w", err)
	}

	// Detect KV version
	if err := backend.detectKVVersion(); err != nil {
		return nil, fmt.Errorf("failed to detect KV version: %w", err)
	}

	return backend, nil
}

// WriteSecret writes a secret to Vault
func (v *VaultBackend) WriteSecret(ctx context.Context, path string, data map[string]any) error {
	fullPath := v.buildPath(path)

	var err error
	if v.isKVv2 {
		// KV v2 requires data wrapped in "data" key
		_, err = v.client.KVv2(v.mountPath).Put(ctx, path, data)
	} else {
		// KV v1 uses direct path
		_, err = v.client.Logical().WriteWithContext(ctx, fullPath, data)
	}

	if err != nil {
		return fmt.Errorf("failed to write secret to vault at %s: %w", fullPath, err)
	}

	return nil
}

// ReadSecret reads a secret from Vault
func (v *VaultBackend) ReadSecret(ctx context.Context, path string) (map[string]any, error) {
	fullPath := v.buildPath(path)

	var secret *vaultapi.KVSecret
	var logicalSecret *vaultapi.Secret
	var err error

	if v.isKVv2 {
		secret, err = v.client.KVv2(v.mountPath).Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to read secret from vault at %s: %w", fullPath, err)
		}
		if secret == nil || secret.Data == nil {
			return nil, fmt.Errorf("secret not found at %s", fullPath)
		}
		return secret.Data, nil
	} else {
		logicalSecret, err = v.client.Logical().ReadWithContext(ctx, fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read secret from vault at %s: %w", fullPath, err)
		}
		if logicalSecret == nil || logicalSecret.Data == nil {
			return nil, fmt.Errorf("secret not found at %s", fullPath)
		}
		return logicalSecret.Data, nil
	}
}

// DeleteSecret deletes a secret from Vault
func (v *VaultBackend) DeleteSecret(ctx context.Context, path string) error {
	fullPath := v.buildPath(path)

	var err error
	if v.isKVv2 {
		// KV v2 uses metadata delete to permanently remove all versions
		err = v.client.KVv2(v.mountPath).DeleteMetadata(ctx, path)
	} else {
		// KV v1 uses logical delete
		_, err = v.client.Logical().DeleteWithContext(ctx, fullPath)
	}

	if err != nil {
		return fmt.Errorf("failed to delete secret from vault at %s: %w", fullPath, err)
	}

	return nil
}

// SecretExists checks if a secret exists at the given path
func (v *VaultBackend) SecretExists(ctx context.Context, path string) (bool, error) {
	_, err := v.ReadSecret(ctx, path)
	if err != nil {
		if strings.Contains(err.Error(), "secret not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Close closes the Vault client
func (v *VaultBackend) Close() error {
	// Vault client doesn't require explicit cleanup
	return nil
}

// buildPath constructs the full Vault path
func (v *VaultBackend) buildPath(path string) string {
	if v.isKVv2 {
		return fmt.Sprintf("%s/data/%s", v.mountPath, path)
	}
	return fmt.Sprintf("%s/%s", v.mountPath, path)
}

// detectKVVersion detects whether the mount is KV v1 or v2
func (v *VaultBackend) detectKVVersion() error {
	// Try to access the mount config to determine version
	mounts, err := v.client.Sys().ListMounts()
	if err != nil {
		return fmt.Errorf("failed to list mounts: %w", err)
	}

	mountPath := v.mountPath + "/"
	mount, ok := mounts[mountPath]
	if !ok {
		return fmt.Errorf("mount %s not found", v.mountPath)
	}

	// Check if it's KV v2
	if mount.Options != nil && mount.Options["version"] == "2" {
		v.isKVv2 = true
	} else if mount.Options != nil && mount.Options["version"] == "1" {
		v.isKVv2 = false
	} else {
		// Default to v2 for KV mounts
		v.isKVv2 = (mount.Type == "kv" || mount.Type == "generic")
	}

	return nil
}
