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

package controller

import (
	"context"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ironcore-dev/bmc-secret-operator/internal/secretbackend"
)

// SecretBackendConfigReconciler reconciles a SecretBackendConfig object
type SecretBackendConfigReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	BackendFactory *secretbackend.BackendFactory
}

// +kubebuilder:rbac:groups=config.metal.ironcore.dev,resources=secretbackendconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.metal.ironcore.dev,resources=secretbackendconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch

// Reconcile handles SecretBackendConfig changes
func (r *SecretBackendConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the SecretBackendConfig
	var config configv1alpha1.SecretBackendConfig
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		if errors.IsNotFound(err) {
			// Config was deleted - operator will fall back to environment variables
			logger.Info("SecretBackendConfig deleted, invalidating cache")
			if err := r.BackendFactory.InvalidateCache(); err != nil {
				logger.Error(err, "Failed to invalidate backend cache")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get SecretBackendConfig")
		return ctrl.Result{}, err
	}

	logger.Info("SecretBackendConfig changed, invalidating cache")

	// Invalidate the backend factory cache
	if err := r.BackendFactory.InvalidateCache(); err != nil {
		logger.Error(err, "Failed to invalidate backend cache")
		return ctrl.Result{}, err
	}

	// Note: We don't need to manually trigger reconciliation of BMCSecrets
	// The operator will naturally reconcile them on the next periodic sync (5 minutes)
	// or when BMC/BMCSecret resources are updated
	// The cache invalidation ensures the next reconciliation uses the new configuration

	logger.Info("Successfully invalidated cache - new config will be used on next reconciliation")
	r.Recorder.Event(&config, "Normal", "ConfigReloaded", "Configuration cache invalidated, new settings will be applied on next sync")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *SecretBackendConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configv1alpha1.SecretBackendConfig{}).
		Complete(r)
}
