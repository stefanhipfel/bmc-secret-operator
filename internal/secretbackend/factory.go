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
	"context"
	"fmt"
	"sync"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
	"github.com/ironcore-dev/bmc-secret-operator/internal/secretbackend/vault"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultBackendConfigName = "default-backend-config"
)

// BackendFactory manages backend instances
type BackendFactory struct {
	client      client.Client
	backend     Backend
	pathBuilder *PathBuilder
	config      *Config
	mu          sync.RWMutex
}

// NewBackendFactory creates a new backend factory
func NewBackendFactory(c client.Client) (*BackendFactory, error) {
	return &BackendFactory{
		client: c,
	}, nil
}

// GetBackend returns the backend instance, initializing if necessary
func (f *BackendFactory) GetBackend(ctx context.Context) (Backend, error) {
	f.mu.RLock()
	if f.backend != nil {
		defer f.mu.RUnlock()
		return f.backend, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock
	if f.backend != nil {
		return f.backend, nil
	}

	// Load configuration
	config, err := f.loadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create backend
	backend, err := f.createBackend(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create backend: %w", err)
	}

	f.backend = backend
	f.config = config

	return backend, nil
}

// GetPathBuilder returns the path builder, initializing if necessary
func (f *BackendFactory) GetPathBuilder(ctx context.Context) (*PathBuilder, error) {
	f.mu.RLock()
	if f.pathBuilder != nil {
		defer f.mu.RUnlock()
		return f.pathBuilder, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock
	if f.pathBuilder != nil {
		return f.pathBuilder, nil
	}

	// Load configuration if not already loaded
	if f.config == nil {
		config, err := f.loadConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration: %w", err)
		}
		f.config = config
	}

	// Create path builder
	pathBuilder, err := NewPathBuilder(f.config.PathTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to create path builder: %w", err)
	}

	f.pathBuilder = pathBuilder
	return pathBuilder, nil
}

// GetRegionLabelKey returns the configured region label key
func (f *BackendFactory) GetRegionLabelKey(ctx context.Context) (string, error) {
	f.mu.RLock()
	if f.config != nil {
		defer f.mu.RUnlock()
		return f.config.RegionLabelKey, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.config == nil {
		config, err := f.loadConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to load configuration: %w", err)
		}
		f.config = config
	}

	return f.config.RegionLabelKey, nil
}

// GetSyncLabel returns the configured sync label key (empty string if not configured)
func (f *BackendFactory) GetSyncLabel(ctx context.Context) (string, error) {
	f.mu.RLock()
	if f.config != nil {
		defer f.mu.RUnlock()
		return f.config.SyncLabel, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.config == nil {
		config, err := f.loadConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to load configuration: %w", err)
		}
		f.config = config
	}

	return f.config.SyncLabel, nil
}

// loadConfig loads configuration from CRD or environment variables
func (f *BackendFactory) loadConfig(ctx context.Context) (*Config, error) {
	// Try to load from CRD first
	var backendConfig configv1alpha1.SecretBackendConfig
	err := f.client.Get(ctx, types.NamespacedName{Name: DefaultBackendConfigName}, &backendConfig)
	if err == nil {
		return LoadConfigFromCRD(&backendConfig)
	}

	// Fall back to environment variables
	return LoadConfigFromEnv()
}

// createBackend creates a backend instance based on configuration
func (f *BackendFactory) createBackend(config *Config) (Backend, error) {
	switch config.Backend {
	case "vault":
		if config.VaultConfig == nil {
			return nil, fmt.Errorf("vault configuration is required when backend is vault")
		}
		// Convert internal config to vault.Config
		vaultConfig := &vault.Config{
			Address:            config.VaultConfig.Address,
			AuthMethod:         config.VaultConfig.AuthMethod,
			KubernetesAuthRole: config.VaultConfig.KubernetesAuthRole,
			KubernetesAuthPath: config.VaultConfig.KubernetesAuthPath,
			Token:              config.VaultConfig.Token,
			MountPath:          config.VaultConfig.MountPath,
			SkipVerify:         config.VaultConfig.SkipVerify,
			CACert:             config.VaultConfig.CACert,
		}
		return vault.NewVaultBackend(vaultConfig)

	case "openbao":
		return nil, fmt.Errorf("OpenBao backend not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported backend type: %s", config.Backend)
	}
}

// Close closes the backend and cleans up resources
func (f *BackendFactory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.backend != nil {
		if err := f.backend.Close(); err != nil {
			return err
		}
		f.backend = nil
	}

	return nil
}

// InvalidateCache invalidates the cached configuration and backend
// This should be called when SecretBackendConfig changes
func (f *BackendFactory) InvalidateCache() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Close existing backend
	if f.backend != nil {
		if err := f.backend.Close(); err != nil {
			return fmt.Errorf("failed to close backend during cache invalidation: %w", err)
		}
		f.backend = nil
	}

	// Clear cached config and path builder
	f.config = nil
	f.pathBuilder = nil

	return nil
}
