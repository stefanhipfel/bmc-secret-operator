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
	"maps"
	"sync"

	"github.com/ironcore-dev/bmc-secret-operator/internal/secretbackend"
)

// MockBackend implements a mock Backend for testing
type MockBackend struct {
	mu      sync.RWMutex
	secrets map[string]map[string]any

	// Track operations for testing
	WriteSecretCalls  []WriteSecretCall
	ReadSecretCalls   []string
	DeleteSecretCalls []string
	SecretExistsCalls []string
	CloseCalled       bool

	// Configure mock behavior
	WriteError        error
	ReadError         error
	DeleteError       error
	SecretExistsError error
}

type WriteSecretCall struct {
	Path string
	Data map[string]any
}

// NewMockBackend creates a new mock backend
func NewMockBackend() *MockBackend {
	return &MockBackend{
		secrets: make(map[string]map[string]any),
	}
}

// WriteSecret writes a secret to the mock backend
func (m *MockBackend) WriteSecret(ctx context.Context, path string, data map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.WriteSecretCalls = append(m.WriteSecretCalls, WriteSecretCall{Path: path, Data: data})

	if m.WriteError != nil {
		return m.WriteError
	}

	// Deep copy data
	dataCopy := make(map[string]any)
	maps.Copy(dataCopy, data)
	m.secrets[path] = dataCopy

	return nil
}

// ReadSecret reads a secret from the mock backend
func (m *MockBackend) ReadSecret(ctx context.Context, path string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.ReadSecretCalls = append(m.ReadSecretCalls, path)

	if m.ReadError != nil {
		return nil, m.ReadError
	}

	data, exists := m.secrets[path]
	if !exists {
		return nil, fmt.Errorf("secret not found at %s", path)
	}

	// Deep copy data
	dataCopy := make(map[string]any)
	maps.Copy(dataCopy, data)

	return dataCopy, nil
}

// DeleteSecret deletes a secret from the mock backend
func (m *MockBackend) DeleteSecret(ctx context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteSecretCalls = append(m.DeleteSecretCalls, path)

	if m.DeleteError != nil {
		return m.DeleteError
	}

	delete(m.secrets, path)
	return nil
}

// SecretExists checks if a secret exists in the mock backend
func (m *MockBackend) SecretExists(ctx context.Context, path string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.SecretExistsCalls = append(m.SecretExistsCalls, path)

	if m.SecretExistsError != nil {
		return false, m.SecretExistsError
	}

	_, exists := m.secrets[path]
	return exists, nil
}

// Close closes the mock backend
func (m *MockBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CloseCalled = true
	return nil
}

// Reset clears all tracked calls and secrets
func (m *MockBackend) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.secrets = make(map[string]map[string]any)
	m.WriteSecretCalls = nil
	m.ReadSecretCalls = nil
	m.DeleteSecretCalls = nil
	m.SecretExistsCalls = nil
	m.CloseCalled = false
	m.WriteError = nil
	m.ReadError = nil
	m.DeleteError = nil
	m.SecretExistsError = nil
}

// GetWriteCallCount returns the number of WriteSecret calls
func (m *MockBackend) GetWriteCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.WriteSecretCalls)
}

// GetDeleteCallCount returns the number of DeleteSecret calls
func (m *MockBackend) GetDeleteCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.DeleteSecretCalls)
}

// GetSecretCount returns the number of secrets stored
func (m *MockBackend) GetSecretCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.secrets)
}

// MockBackendFactory creates a factory that returns the mock backend
type MockBackendFactory struct {
	Backend        *MockBackend
	PathBuilder    *secretbackend.PathBuilder
	RegionLabelKey string
	SyncLabel      string
	GetBackendErr  error
}

func NewMockBackendFactory(mockBackend *MockBackend, pathTemplate, regionLabelKey, syncLabel string) (*MockBackendFactory, error) {
	pathBuilder, err := secretbackend.NewPathBuilder(pathTemplate)
	if err != nil {
		return nil, err
	}

	return &MockBackendFactory{
		Backend:        mockBackend,
		PathBuilder:    pathBuilder,
		RegionLabelKey: regionLabelKey,
		SyncLabel:      syncLabel,
	}, nil
}

func (m *MockBackendFactory) GetBackend(ctx context.Context) (secretbackend.Backend, error) {
	if m.GetBackendErr != nil {
		return nil, m.GetBackendErr
	}
	return m.Backend, nil
}

func (m *MockBackendFactory) GetPathBuilder(ctx context.Context) (*secretbackend.PathBuilder, error) {
	return m.PathBuilder, nil
}

func (m *MockBackendFactory) GetRegionLabelKey(ctx context.Context) (string, error) {
	return m.RegionLabelKey, nil
}

func (m *MockBackendFactory) GetSyncLabel(ctx context.Context) (string, error) {
	return m.SyncLabel, nil
}

func (m *MockBackendFactory) Close() error {
	return m.Backend.Close()
}

func (m *MockBackendFactory) InvalidateCache() error {
	// No-op for mock - config changes are handled by updating the mock fields directly
	return nil
}
