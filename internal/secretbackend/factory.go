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
	"time"

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
	client           client.Client
	backend          Backend
	pathBuilder      *PathBuilder
	config           *Config
	metricsCollector MetricsCollector
	engineBackends   []*EngineBackend
	mu               sync.RWMutex
}

// MetricsCollector defines the interface for recording metrics
// This allows the factory to be independent of the metrics implementation
type MetricsCollector interface {
	RecordAuth(method, backendType string, duration time.Duration, err error)
}

// NewBackendFactory creates a new backend factory
func NewBackendFactory(c client.Client, metricsCollector MetricsCollector) (*BackendFactory, error) {
	return &BackendFactory{
		client:           c,
		metricsCollector: metricsCollector,
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
	var backend Backend
	var err error

	switch config.Backend {
	case defaultBackendType:
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
		backend, err = vault.NewVaultBackend(vaultConfig, f.metricsCollector)

	case "openbao":
		return nil, fmt.Errorf("OpenBao backend not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported backend type: %s", config.Backend)
	}

	if err != nil {
		return nil, err
	}

	// Wrap backend with metrics instrumentation if metrics collector is available
	if f.metricsCollector != nil {
		backend = newInstrumentedBackend(backend, config.Backend, f.metricsCollector)
	}

	return backend, nil
}

// newInstrumentedBackend wraps a backend with metrics instrumentation
func newInstrumentedBackend(backend Backend, backendType string, collector MetricsCollector) Backend {
	return &instrumentedBackend{
		backend:     backend,
		backendType: backendType,
		collector:   collector,
	}
}

// newInstrumentedBackendWithEngine wraps a backend with metrics instrumentation including engine name
func newInstrumentedBackendWithEngine(backend Backend, backendType, engineName string, collector MetricsCollector) Backend {
	return &instrumentedBackendWithEngine{
		backend:     backend,
		backendType: backendType,
		engineName:  engineName,
		collector:   collector,
	}
}

// instrumentedBackendWithEngine wraps a Backend with metrics instrumentation including engine name
type instrumentedBackendWithEngine struct {
	backend     Backend
	backendType string
	engineName  string
	collector   MetricsCollector
}

// WriteSecret writes a secret and records metrics
func (i *instrumentedBackendWithEngine) WriteSecret(ctx context.Context, path string, data map[string]any) error {
	start := time.Now()
	err := i.backend.WriteSecret(ctx, path, data)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperationWithEngine(operation, backendType, engine string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperationWithEngine("write", i.backendType, i.engineName, duration, err)
	} else if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("write", i.backendType, duration, err)
	}
	return err
}

// ReadSecret reads a secret and records metrics
func (i *instrumentedBackendWithEngine) ReadSecret(ctx context.Context, path string) (map[string]any, error) {
	start := time.Now()
	data, err := i.backend.ReadSecret(ctx, path)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperationWithEngine(operation, backendType, engine string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperationWithEngine("read", i.backendType, i.engineName, duration, err)
	} else if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("read", i.backendType, duration, err)
	}
	return data, err
}

// DeleteSecret deletes a secret and records metrics
func (i *instrumentedBackendWithEngine) DeleteSecret(ctx context.Context, path string) error {
	start := time.Now()
	err := i.backend.DeleteSecret(ctx, path)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperationWithEngine(operation, backendType, engine string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperationWithEngine("delete", i.backendType, i.engineName, duration, err)
	} else if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("delete", i.backendType, duration, err)
	}
	return err
}

// SecretExists checks if a secret exists and records metrics
func (i *instrumentedBackendWithEngine) SecretExists(ctx context.Context, path string) (bool, error) {
	start := time.Now()
	exists, err := i.backend.SecretExists(ctx, path)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperationWithEngine(operation, backendType, engine string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperationWithEngine("exists", i.backendType, i.engineName, duration, err)
	} else if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("exists", i.backendType, duration, err)
	}
	return exists, err
}

// Close closes the backend
func (i *instrumentedBackendWithEngine) Close() error {
	return i.backend.Close()
}

// instrumentedBackend wraps a Backend with metrics instrumentation
type instrumentedBackend struct {
	backend     Backend
	backendType string
	collector   MetricsCollector
}

// WriteSecret writes a secret and records metrics
func (i *instrumentedBackend) WriteSecret(ctx context.Context, path string, data map[string]any) error {
	start := time.Now()
	err := i.backend.WriteSecret(ctx, path, data)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("write", i.backendType, duration, err)
	}
	return err
}

// ReadSecret reads a secret and records metrics
func (i *instrumentedBackend) ReadSecret(ctx context.Context, path string) (map[string]any, error) {
	start := time.Now()
	data, err := i.backend.ReadSecret(ctx, path)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("read", i.backendType, duration, err)
	}
	return data, err
}

// DeleteSecret deletes a secret and records metrics
func (i *instrumentedBackend) DeleteSecret(ctx context.Context, path string) error {
	start := time.Now()
	err := i.backend.DeleteSecret(ctx, path)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("delete", i.backendType, duration, err)
	}
	return err
}

// SecretExists checks if a secret exists and records metrics
func (i *instrumentedBackend) SecretExists(ctx context.Context, path string) (bool, error) {
	start := time.Now()
	exists, err := i.backend.SecretExists(ctx, path)
	duration := time.Since(start)

	if mc, ok := i.collector.(interface {
		RecordBackendOperation(operation, backendType string, duration time.Duration, err error)
	}); ok {
		mc.RecordBackendOperation("exists", i.backendType, duration, err)
	}
	return exists, err
}

// Close closes the backend
func (i *instrumentedBackend) Close() error {
	return i.backend.Close()
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

	// Close engine backends
	for _, eb := range f.engineBackends {
		if eb.Backend != nil {
			if err := eb.Backend.Close(); err != nil {
				return fmt.Errorf("failed to close engine backend %s: %w", eb.EngineName, err)
			}
		}
	}
	f.engineBackends = nil

	// Clear cached config and path builder
	f.config = nil
	f.pathBuilder = nil

	return nil
}

// GetEngineBackends returns engine backends that match the given labels
// If no secret engines are configured, returns empty slice (backward compatibility)
func (f *BackendFactory) GetEngineBackends(ctx context.Context, labels map[string]string) ([]*EngineBackend, error) {
	f.mu.RLock()
	if f.engineBackends != nil {
		defer f.mu.RUnlock()
		return f.filterEnginesByLabels(f.engineBackends, labels), nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock
	if f.engineBackends != nil {
		return f.filterEnginesByLabels(f.engineBackends, labels), nil
	}

	// Load configuration if not already loaded
	if f.config == nil {
		config, err := f.loadConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration: %w", err)
		}
		f.config = config
	}

	// Check if multi-engine configuration exists
	if f.config.VaultConfig == nil {
		return nil, nil
	}

	// Get secret engines from CRD
	var engines []configv1alpha1.SecretEngineConfig
	var backendConfig configv1alpha1.SecretBackendConfig
	err := f.client.Get(ctx, types.NamespacedName{Name: DefaultBackendConfigName}, &backendConfig)
	if err != nil {
		// No CRD config, return empty list
		return nil, nil
	}

	if backendConfig.Spec.VaultConfig != nil {
		engines = backendConfig.Spec.VaultConfig.SecretEngines
	}

	if len(engines) == 0 {
		// No secret engines configured
		return nil, nil
	}

	// Create engine backends
	engineBackends, err := parseSecretEngineConfig(engines, f.config.VaultConfig, f.metricsCollector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse secret engine config: %w", err)
	}

	// Wrap each backend with metrics instrumentation
	for _, eb := range engineBackends {
		if f.metricsCollector != nil {
			eb.Backend = newInstrumentedBackendWithEngine(eb.Backend, f.config.Backend, eb.EngineName, f.metricsCollector)
		}
	}

	f.engineBackends = engineBackends
	return f.filterEnginesByLabels(engineBackends, labels), nil
}

// filterEnginesByLabels filters engine backends by label matching
func (f *BackendFactory) filterEnginesByLabels(engines []*EngineBackend, labels map[string]string) []*EngineBackend {
	var matched []*EngineBackend
	for _, engine := range engines {
		if engine.MatchesLabels(labels) {
			matched = append(matched, engine)
		}
	}
	return matched
}

// HasMultiEngineConfig checks if multi-engine configuration is present
func (f *BackendFactory) HasMultiEngineConfig(ctx context.Context) (bool, error) {
	var backendConfig configv1alpha1.SecretBackendConfig
	err := f.client.Get(ctx, types.NamespacedName{Name: DefaultBackendConfigName}, &backendConfig)
	if err != nil {
		return false, nil
	}

	if backendConfig.Spec.VaultConfig != nil {
		return len(backendConfig.Spec.VaultConfig.SecretEngines) > 0, nil
	}

	return false, nil
}
