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
)

// Backend defines the interface for secret backend operations
type Backend interface {
	// WriteSecret writes a secret to the backend at the specified path
	WriteSecret(ctx context.Context, path string, data map[string]any) error

	// ReadSecret reads a secret from the backend at the specified path
	ReadSecret(ctx context.Context, path string) (map[string]any, error)

	// DeleteSecret deletes a secret from the backend at the specified path
	DeleteSecret(ctx context.Context, path string) error

	// SecretExists checks if a secret exists at the specified path
	SecretExists(ctx context.Context, path string) (bool, error)

	// Close cleans up backend resources
	Close() error
}

// BackendFactoryInterface defines the interface for backend factory operations
type BackendFactoryInterface interface {
	// GetBackend returns the backend instance
	GetBackend(ctx context.Context) (Backend, error)

	// GetPathBuilder returns the path builder
	GetPathBuilder(ctx context.Context) (*PathBuilder, error)

	// GetRegionLabelKey returns the configured region label key
	GetRegionLabelKey(ctx context.Context) (string, error)

	// GetSyncLabel returns the configured sync label key
	GetSyncLabel(ctx context.Context) (string, error)

	// GetEngineBackends returns engine backends that match the given labels
	GetEngineBackends(ctx context.Context, labels map[string]string) ([]*EngineBackend, error)

	// HasMultiEngineConfig checks if multi-engine configuration is present
	HasMultiEngineConfig(ctx context.Context) (bool, error)

	// InvalidateCache invalidates cached configuration and backend
	InvalidateCache() error

	// Close closes the backend and cleans up resources
	Close() error
}
