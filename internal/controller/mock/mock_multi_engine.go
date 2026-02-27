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

package mock

import (
	"context"
	"fmt"
	"sync"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
	"github.com/ironcore-dev/bmc-secret-operator/internal/secretbackend"
)

// MultiEngineBackendFactory manages multiple backend instances for different engines
type MultiEngineBackendFactory struct {
	mu              sync.RWMutex
	backends        map[string]*MockBackend
	engines         []configv1alpha1.SecretEngineConfig
	globalSyncLabel string
	pathBuilders    map[string]*secretbackend.PathBuilder
	regionLabelKey  string
	GetBackendErr   error
	GetEngineErr    error
}

// NewMultiEngineBackendFactory creates a factory supporting multiple engines
func NewMultiEngineBackendFactory(
	engines []configv1alpha1.SecretEngineConfig,
	globalSyncLabel string,
	regionLabelKey string,
) (*MultiEngineBackendFactory, error) {
	factory := &MultiEngineBackendFactory{
		backends:        make(map[string]*MockBackend),
		engines:         engines,
		globalSyncLabel: globalSyncLabel,
		pathBuilders:    make(map[string]*secretbackend.PathBuilder),
		regionLabelKey:  regionLabelKey,
	}

	// Create backends and path builders for each engine
	for _, engine := range engines {
		factory.backends[engine.Name] = NewMockBackend()

		pathTemplate := engine.PathTemplate
		if pathTemplate == "" {
			pathTemplate = "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
		}

		pathBuilder, err := secretbackend.NewPathBuilder(pathTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to create path builder for engine %s: %w", engine.Name, err)
		}
		factory.pathBuilders[engine.Name] = pathBuilder
	}

	return factory, nil
}

// GetBackend returns the default/first backend (for backward compatibility)
func (f *MultiEngineBackendFactory) GetBackend(ctx context.Context) (secretbackend.Backend, error) {
	if f.GetBackendErr != nil {
		return nil, f.GetBackendErr
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.backends) == 0 {
		return nil, fmt.Errorf("no backends available")
	}

	// Return the first backend for backward compatibility
	for _, backend := range f.backends {
		return backend, nil
	}
	return nil, fmt.Errorf("no backends available")
}

// GetPathBuilder returns the path builder for a specific engine
func (f *MultiEngineBackendFactory) GetPathBuilder(ctx context.Context) (*secretbackend.PathBuilder, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.pathBuilders) == 0 {
		return nil, fmt.Errorf("no path builders available")
	}

	// Return the first path builder for backward compatibility
	for _, builder := range f.pathBuilders {
		return builder, nil
	}
	return nil, fmt.Errorf("no path builders available")
}

// GetPathBuilderForEngine returns the path builder for a specific engine
func (f *MultiEngineBackendFactory) GetPathBuilderForEngine(ctx context.Context, engineName string) (*secretbackend.PathBuilder, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	builder, exists := f.pathBuilders[engineName]
	if !exists {
		return nil, fmt.Errorf("path builder not found for engine %s", engineName)
	}
	return builder, nil
}

// GetBackendForEngine returns the backend for a specific engine
func (f *MultiEngineBackendFactory) GetBackendForEngine(ctx context.Context, engineName string) (secretbackend.Backend, error) {
	if f.GetBackendErr != nil {
		return nil, f.GetBackendErr
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	backend, exists := f.backends[engineName]
	if !exists {
		return nil, fmt.Errorf("backend not found for engine %s", engineName)
	}
	return backend, nil
}

// GetMatchingEngines returns all engines that match the given labels
func (f *MultiEngineBackendFactory) GetMatchingEngines(ctx context.Context, labels map[string]string) ([]configv1alpha1.SecretEngineConfig, error) {
	if f.GetEngineErr != nil {
		return nil, f.GetEngineErr
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	var matching []configv1alpha1.SecretEngineConfig

	// If global syncLabel is set, check it first
	if f.globalSyncLabel != "" {
		if _, exists := labels[f.globalSyncLabel]; !exists {
			return matching, nil
		}
	}

	// Check each engine's syncLabel
	for _, engine := range f.engines {
		if matchesLabel(labels, engine.SyncLabel) {
			matching = append(matching, engine)
		}
	}

	return matching, nil
}

// matchesLabel checks if a label map matches the label spec
// Spec format: "key" or "key=value"
// "key" matches if the key exists with any value
// "key=value" matches if the key exists with exactly that value
func matchesLabel(labels map[string]string, labelSpec string) bool {
	if labelSpec == "" {
		return false
	}

	// Parse label spec
	var key, expectedValue string
	for i, ch := range labelSpec {
		if ch == '=' {
			key = labelSpec[:i]
			expectedValue = labelSpec[i+1:]
			break
		}
	}

	if key == "" {
		key = labelSpec
	}

	value, exists := labels[key]
	if !exists {
		return false
	}

	// If no expected value is specified, any value matches
	if expectedValue == "" {
		return true
	}

	return value == expectedValue
}

// GetRegionLabelKey returns the configured region label key
func (f *MultiEngineBackendFactory) GetRegionLabelKey(ctx context.Context) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.regionLabelKey, nil
}

// GetSyncLabel returns the global sync label
func (f *MultiEngineBackendFactory) GetSyncLabel(ctx context.Context) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.globalSyncLabel, nil
}

// GetSecretEngines returns all configured secret engines
func (f *MultiEngineBackendFactory) GetSecretEngines(ctx context.Context) ([]configv1alpha1.SecretEngineConfig, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.engines, nil
}

// Close closes all backends
func (f *MultiEngineBackendFactory) Close() error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for engineName, backend := range f.backends {
		if err := backend.Close(); err != nil {
			return fmt.Errorf("failed to close backend for engine %s: %w", engineName, err)
		}
	}
	return nil
}

// InvalidateCache is a no-op for the mock factory
func (f *MultiEngineBackendFactory) InvalidateCache() error {
	return nil
}

// GetEngineBackends returns engine backends that match the given labels
func (f *MultiEngineBackendFactory) GetEngineBackends(ctx context.Context, labels map[string]string) ([]*secretbackend.EngineBackend, error) {
	if f.GetEngineErr != nil {
		return nil, f.GetEngineErr
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	var engineBackends []*secretbackend.EngineBackend

	// Check each engine's syncLabel
	for _, engine := range f.engines {
		if !matchesLabel(labels, engine.SyncLabel) {
			continue
		}

		// Get backend for this engine
		backend, exists := f.backends[engine.Name]
		if !exists {
			continue
		}

		// Get path builder for this engine
		pathBuilder, exists := f.pathBuilders[engine.Name]
		if !exists {
			continue
		}

		// Parse sync label
		syncLabelKey, syncLabelVal := parseSyncLabel(engine.SyncLabel)

		engineBackend := &secretbackend.EngineBackend{
			Backend:      backend,
			EngineName:   engine.Name,
			PathBuilder:  pathBuilder,
			SyncLabel:    engine.SyncLabel,
			SyncLabelKey: syncLabelKey,
			SyncLabelVal: syncLabelVal,
		}

		engineBackends = append(engineBackends, engineBackend)
	}

	return engineBackends, nil
}

// HasMultiEngineConfig checks if multi-engine configuration is present
func (f *MultiEngineBackendFactory) HasMultiEngineConfig(ctx context.Context) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.engines) > 0, nil
}

// parseSyncLabel splits a sync label into key and value
func parseSyncLabel(syncLabel string) (string, string) {
	for i, ch := range syncLabel {
		if ch == '=' {
			return syncLabel[:i], syncLabel[i+1:]
		}
	}
	return syncLabel, ""
}

// GetMockBackendForEngine returns the underlying MockBackend for assertions in tests
func (f *MultiEngineBackendFactory) GetMockBackendForEngine(engineName string) *MockBackend {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.backends[engineName]
}

// GetAllMockBackends returns all underlying MockBackends
func (f *MultiEngineBackendFactory) GetAllMockBackends() map[string]*MockBackend {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Create a copy of the map
	result := make(map[string]*MockBackend, len(f.backends))
	for k := range f.backends {
		result[k] = f.backends[k]
	}
	return result
}
