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
	"fmt"
	"time"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/ironcore-dev/bmc-secret-operator/internal/controller/bmcresolver"
	"github.com/ironcore-dev/bmc-secret-operator/internal/metrics"
	"github.com/ironcore-dev/bmc-secret-operator/internal/secretbackend"
)

const (
	bmcSecretFinalizer = "bmcsecret.metal.ironcore.dev/backend-cleanup"
	requeueAfterNormal = 5 * time.Minute
	requeueAfterError  = 30 * time.Second

	reconcileResultSuccess = "success"
	reconcileResultError   = "error"
)

// BMCSecretReconciler reconciles a BMCSecret object
type BMCSecretReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	BackendFactory secretbackend.BackendFactoryInterface
	Metrics        *metrics.Collector
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.metal.ironcore.dev,resources=secretbackendconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.metal.ironcore.dev,resources=bmcsecretsyncstatuses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.metal.ironcore.dev,resources=bmcsecretsyncstatuses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop
func (r *BMCSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	startTime := time.Now()
	var reconcileErr error

	defer func() {
		if r.Metrics != nil {
			duration := time.Since(startTime)
			r.Metrics.RecordReconcileDuration("reconcile", duration)
			reconcileResult := reconcileResultSuccess
			if reconcileErr != nil {
				reconcileResult = reconcileResultError
			}
			r.Metrics.RecordReconcileResult(reconcileResult)
		}
	}()

	// Fetch the BMCSecret
	var bmcSecret metalv1alpha1.BMCSecret
	if err := r.Get(ctx, req.NamespacedName, &bmcSecret); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get BMCSecret")
		reconcileErr = err
		return ctrl.Result{}, err
	}

	// Check if secret should be synced based on label
	syncLabel, err := r.BackendFactory.GetSyncLabel(ctx)
	if err != nil {
		logger.Error(err, "Failed to get sync label configuration")
		reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	// If sync label is configured, check if the BMCSecret has it
	if syncLabel != "" {
		if bmcSecret.Labels == nil || bmcSecret.Labels[syncLabel] == "" {
			logger.V(1).Info("BMCSecret does not have required sync label, skipping", "syncLabel", syncLabel)
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion
	if !bmcSecret.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &bmcSecret)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&bmcSecret, bmcSecretFinalizer) {
		controllerutil.AddFinalizer(&bmcSecret, bmcSecretFinalizer)
		if err := r.Update(ctx, &bmcSecret); err != nil {
			logger.Error(err, "Failed to add finalizer")
			reconcileErr = err
			return ctrl.Result{}, err
		}
	}

	// Discover BMCs that reference this secret
	bmcs, err := bmcresolver.FindBMCsForSecret(ctx, r.Client, bmcSecret.Name)
	if err != nil {
		logger.Error(err, "Failed to find BMCs for secret")
		r.Recorder.Event(&bmcSecret, "Warning", "BMCDiscoveryFailed", err.Error())
		reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	if r.Metrics != nil {
		r.Metrics.RecordBMCCount(bmcSecret.Name, len(bmcs))
	}

	if len(bmcs) == 0 {
		logger.Info("No BMCs reference this secret")
		r.Recorder.Event(&bmcSecret, "Normal", "NoBMCReference", "No BMCs reference this secret")
		return ctrl.Result{RequeueAfter: requeueAfterNormal}, nil
	}

	// Extract credentials
	username, password, err := bmcresolver.ExtractCredentials(&bmcSecret)
	if r.Metrics != nil {
		r.Metrics.RecordCredentialExtraction(bmcSecret.Name, err)
	}
	if err != nil {
		logger.Error(err, "Failed to extract credentials")
		r.Recorder.Event(&bmcSecret, "Warning", "MissingCredentials", err.Error())
		reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	// Check if multi-engine configuration exists
	hasMultiEngine, err := r.BackendFactory.HasMultiEngineConfig(ctx)
	if err != nil {
		logger.Error(err, "Failed to check multi-engine configuration")
		reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	if hasMultiEngine {
		// Use multi-engine sync path
		return r.reconcileMultiEngine(ctx, &bmcSecret, bmcs, username, password, &reconcileErr)
	}

	// Fall back to single-engine path for backward compatibility
	return r.reconcileSingleEngine(ctx, &bmcSecret, bmcs, username, password, &reconcileErr)
}

// reconcileSingleEngine handles reconciliation for single-engine configuration (backward compatibility)
func (r *BMCSecretReconciler) reconcileSingleEngine(
	ctx context.Context,
	bmcSecret *metalv1alpha1.BMCSecret,
	bmcs []metalv1alpha1.BMC,
	username, password string,
	reconcileErr *error,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get backend
	backend, err := r.BackendFactory.GetBackend(ctx)
	if err != nil {
		logger.Error(err, "Failed to get backend")
		r.Recorder.Event(bmcSecret, "Warning", "BackendUnavailable", err.Error())
		*reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	// Get path builder
	pathBuilder, err := r.BackendFactory.GetPathBuilder(ctx)
	if err != nil {
		logger.Error(err, "Failed to get path builder")
		*reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	// Get region label key
	regionLabelKey, err := r.BackendFactory.GetRegionLabelKey(ctx)
	if err != nil {
		logger.Error(err, "Failed to get region label key")
		*reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	// Sync secrets for each BMC and track status
	syncErrors := 0
	syncSuccess := 0
	backendPaths := make([]configv1alpha1.BackendPath, 0, len(bmcs))
	syncTime := metav1.Now()

	for _, bmc := range bmcs {
		region := bmcresolver.ExtractRegionFromBMC(&bmc, regionLabelKey)
		hostname := bmcresolver.GetHostnameFromBMC(&bmc)

		// Build path
		path, err := pathBuilder.Build(secretbackend.PathVariables{
			Region:   region,
			Hostname: hostname,
			Username: username,
		})
		if err != nil {
			logger.Error(err, "Failed to build path", "bmc", bmc.Name)
			backendPaths = append(backendPaths, configv1alpha1.BackendPath{
				Path:         path,
				BMCName:      bmc.Name,
				Region:       region,
				Hostname:     hostname,
				Username:     username,
				LastSyncTime: syncTime,
				SyncStatus:   "Failed",
				ErrorMessage: err.Error(),
			})
			syncErrors++
			continue
		}

		// Check if update needed
		needsUpdate, err := r.needsUpdate(ctx, backend, path, password)
		if err != nil {
			logger.Error(err, "Failed to check if update needed", "path", path)
			backendPaths = append(backendPaths, configv1alpha1.BackendPath{
				Path:         path,
				BMCName:      bmc.Name,
				Region:       region,
				Hostname:     hostname,
				Username:     username,
				LastSyncTime: syncTime,
				SyncStatus:   "Failed",
				ErrorMessage: err.Error(),
			})
			syncErrors++
			continue
		}

		if !needsUpdate {
			logger.V(1).Info("Secret already up to date", "path", path)
			backendPaths = append(backendPaths, configv1alpha1.BackendPath{
				Path:         path,
				BMCName:      bmc.Name,
				Region:       region,
				Hostname:     hostname,
				Username:     username,
				LastSyncTime: syncTime,
				SyncStatus:   "Success",
			})
			syncSuccess++
			continue
		}

		// Write to backend
		secretData := map[string]any{
			"username": username,
			"password": password,
		}

		if err := backend.WriteSecret(ctx, path, secretData); err != nil {
			logger.Error(err, "Failed to write secret to backend", "path", path)
			r.Recorder.Eventf(bmcSecret, "Warning", "SyncFailed", "Failed to sync to %s: %v", path, err)
			backendPaths = append(backendPaths, configv1alpha1.BackendPath{
				Path:         path,
				BMCName:      bmc.Name,
				Region:       region,
				Hostname:     hostname,
				Username:     username,
				LastSyncTime: syncTime,
				SyncStatus:   "Failed",
				ErrorMessage: err.Error(),
			})
			syncErrors++
			continue
		}

		logger.Info("Successfully synced secret", "path", path)
		backendPaths = append(backendPaths, configv1alpha1.BackendPath{
			Path:         path,
			BMCName:      bmc.Name,
			Region:       region,
			Hostname:     hostname,
			Username:     username,
			LastSyncTime: syncTime,
			SyncStatus:   "Success",
		})
		syncSuccess++
	}

	// Update BMCSecretSyncStatus
	if err := r.updateSyncStatus(ctx, bmcSecret.Name, backendPaths, len(bmcs), syncSuccess, syncErrors); err != nil {
		logger.Error(err, "Failed to update sync status")
		// Don't fail reconciliation if status update fails
	}

	// Update status
	if syncErrors > 0 {
		r.Recorder.Eventf(bmcSecret, "Warning", "PartialSync", "Synced %d/%d secrets", syncSuccess, len(bmcs))
	} else {
		r.Recorder.Eventf(bmcSecret, "Normal", "Synced", "Successfully synced to %d backend paths", syncSuccess)
	}

	logger.Info("Reconciliation complete", "syncSuccess", syncSuccess, "syncErrors", syncErrors, "totalBMCs", len(bmcs))

	if r.Metrics != nil {
		r.Metrics.RecordSyncStatus(bmcSecret.Name, syncSuccess, syncErrors, syncTime.Time)
	}

	return ctrl.Result{RequeueAfter: requeueAfterNormal}, nil
}

// reconcileMultiEngine handles reconciliation for multi-engine configuration
func (r *BMCSecretReconciler) reconcileMultiEngine(
	ctx context.Context,
	bmcSecret *metalv1alpha1.BMCSecret,
	bmcs []metalv1alpha1.BMC,
	username, password string,
	reconcileErr *error,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get region label key
	regionLabelKey, err := r.BackendFactory.GetRegionLabelKey(ctx)
	if err != nil {
		logger.Error(err, "Failed to get region label key")
		*reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	// Get matching engine backends based on BMCSecret labels
	engineBackends, err := r.BackendFactory.GetEngineBackends(ctx, bmcSecret.Labels)
	if err != nil {
		logger.Error(err, "Failed to get engine backends")
		r.Recorder.Event(bmcSecret, "Warning", "BackendUnavailable", err.Error())
		*reconcileErr = err
		return ctrl.Result{RequeueAfter: requeueAfterError}, err
	}

	if len(engineBackends) == 0 {
		logger.Info("No matching secret engines found for BMCSecret labels", "labels", bmcSecret.Labels)
		r.Recorder.Event(bmcSecret, "Normal", "NoMatchingEngines", "No secret engines match this BMCSecret's labels")
		return ctrl.Result{RequeueAfter: requeueAfterNormal}, nil
	}

	logger.Info("Found matching secret engines", "count", len(engineBackends))

	// Sync to all matching engines
	syncErrors := 0
	syncSuccess := 0
	backendPaths := make([]configv1alpha1.BackendPath, 0, len(bmcs)*len(engineBackends))
	syncTime := metav1.Now()

	secretData := map[string]any{
		"username": username,
		"password": password,
	}

	for _, engineBackend := range engineBackends {
		logger.Info("Syncing to engine", "engine", engineBackend.EngineName)

		for _, bmc := range bmcs {
			region := bmcresolver.ExtractRegionFromBMC(&bmc, regionLabelKey)
			hostname := bmcresolver.GetHostnameFromBMC(&bmc)

			// Build path using engine's path builder
			path, err := engineBackend.PathBuilder.Build(secretbackend.PathVariables{
				Region:   region,
				Hostname: hostname,
				Username: username,
			})
			if err != nil {
				logger.Error(err, "Failed to build path", "bmc", bmc.Name, "engine", engineBackend.EngineName)
				backendPaths = append(backendPaths, configv1alpha1.BackendPath{
					Path:         path,
					BMCName:      bmc.Name,
					Region:       region,
					Hostname:     hostname,
					Username:     username,
					LastSyncTime: syncTime,
					SyncStatus:   "Failed",
					ErrorMessage: fmt.Sprintf("[%s] %s", engineBackend.EngineName, err.Error()),
				})
				syncErrors++
				continue
			}

			// Check if update needed
			needsUpdate, err := r.needsUpdate(ctx, engineBackend.Backend, path, password)
			if err != nil {
				logger.Error(err, "Failed to check if update needed", "path", path, "engine", engineBackend.EngineName)
				backendPaths = append(backendPaths, configv1alpha1.BackendPath{
					Path:         path,
					BMCName:      bmc.Name,
					Region:       region,
					Hostname:     hostname,
					Username:     username,
					LastSyncTime: syncTime,
					SyncStatus:   "Failed",
					ErrorMessage: fmt.Sprintf("[%s] %s", engineBackend.EngineName, err.Error()),
				})
				syncErrors++
				continue
			}

			if !needsUpdate {
				logger.V(1).Info("Secret already up to date", "path", path, "engine", engineBackend.EngineName)
				backendPaths = append(backendPaths, configv1alpha1.BackendPath{
					Path:         path,
					BMCName:      bmc.Name,
					Region:       region,
					Hostname:     hostname,
					Username:     username,
					LastSyncTime: syncTime,
					SyncStatus:   "Success",
				})
				syncSuccess++
				continue
			}

			// Write to backend
			if err := engineBackend.Backend.WriteSecret(ctx, path, secretData); err != nil {
				logger.Error(err, "Failed to write secret to backend", "path", path, "engine", engineBackend.EngineName)
				r.Recorder.Eventf(bmcSecret, "Warning", "SyncFailed", "Failed to sync to %s (engine %s): %v", path, engineBackend.EngineName, err)
				backendPaths = append(backendPaths, configv1alpha1.BackendPath{
					Path:         path,
					BMCName:      bmc.Name,
					Region:       region,
					Hostname:     hostname,
					Username:     username,
					LastSyncTime: syncTime,
					SyncStatus:   "Failed",
					ErrorMessage: fmt.Sprintf("[%s] %s", engineBackend.EngineName, err.Error()),
				})
				syncErrors++
				continue
			}

			logger.Info("Successfully synced secret", "path", path, "engine", engineBackend.EngineName)
			backendPaths = append(backendPaths, configv1alpha1.BackendPath{
				Path:         path,
				BMCName:      bmc.Name,
				Region:       region,
				Hostname:     hostname,
				Username:     username,
				LastSyncTime: syncTime,
				SyncStatus:   "Success",
			})
			syncSuccess++
		}
	}

	// Update BMCSecretSyncStatus
	if err := r.updateSyncStatus(ctx, bmcSecret.Name, backendPaths, len(backendPaths), syncSuccess, syncErrors); err != nil {
		logger.Error(err, "Failed to update sync status")
		// Don't fail reconciliation if status update fails
	}

	// Update status
	if syncErrors > 0 {
		r.Recorder.Eventf(bmcSecret, "Warning", "PartialSync", "Synced %d/%d secrets across %d engines", syncSuccess, len(backendPaths), len(engineBackends))
	} else {
		r.Recorder.Eventf(bmcSecret, "Normal", "Synced", "Successfully synced to %d backend paths across %d engines", syncSuccess, len(engineBackends))
	}

	logger.Info("Multi-engine reconciliation complete", "syncSuccess", syncSuccess, "syncErrors", syncErrors, "totalPaths", len(backendPaths), "engines", len(engineBackends))

	if r.Metrics != nil {
		r.Metrics.RecordSyncStatus(bmcSecret.Name, syncSuccess, syncErrors, syncTime.Time)
	}

	return ctrl.Result{RequeueAfter: requeueAfterNormal}, nil
}

// handleDeletion handles cleanup when BMCSecret is being deleted
func (r *BMCSecretReconciler) handleDeletion(ctx context.Context, bmcSecret *metalv1alpha1.BMCSecret) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	start := time.Now()
	defer func() {
		if r.Metrics != nil {
			r.Metrics.RecordReconcileDuration("deletion", time.Since(start))
		}
	}()

	if !controllerutil.ContainsFinalizer(bmcSecret, bmcSecretFinalizer) {
		return ctrl.Result{}, nil
	}

	logger.Info("Cleaning up backend secrets")

	// Delete corresponding BMCSecretSyncStatus
	syncStatus := &configv1alpha1.BMCSecretSyncStatus{}
	syncStatusName := fmt.Sprintf("%s-sync-status", bmcSecret.Name)
	if err := r.Get(ctx, types.NamespacedName{Name: syncStatusName}, syncStatus); err == nil {
		if err := r.Delete(ctx, syncStatus); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete BMCSecretSyncStatus")
			// Continue with cleanup even if status deletion fails
		}
	}

	// Get backend
	backend, err := r.BackendFactory.GetBackend(ctx)
	if err != nil {
		logger.Error(err, "Failed to get backend during cleanup, allowing deletion to proceed")
		r.Recorder.Event(bmcSecret, "Warning", "CleanupFailed", "Backend unavailable during cleanup")
		// Allow deletion to proceed even if backend is unavailable
		controllerutil.RemoveFinalizer(bmcSecret, bmcSecretFinalizer)
		if err := r.Update(ctx, bmcSecret); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Get path builder and region label key
	pathBuilder, err := r.BackendFactory.GetPathBuilder(ctx)
	if err != nil {
		logger.Error(err, "Failed to get path builder during cleanup")
		// Continue with deletion
		controllerutil.RemoveFinalizer(bmcSecret, bmcSecretFinalizer)
		if err := r.Update(ctx, bmcSecret); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	regionLabelKey, err := r.BackendFactory.GetRegionLabelKey(ctx)
	if err != nil {
		logger.Error(err, "Failed to get region label key during cleanup")
		// Continue with deletion
		controllerutil.RemoveFinalizer(bmcSecret, bmcSecretFinalizer)
		if err := r.Update(ctx, bmcSecret); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Extract credentials
	username, _, err := bmcresolver.ExtractCredentials(bmcSecret)
	if err != nil {
		logger.Error(err, "Failed to extract credentials during cleanup, proceeding with deletion")
		controllerutil.RemoveFinalizer(bmcSecret, bmcSecretFinalizer)
		if err := r.Update(ctx, bmcSecret); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Find associated BMCs
	bmcs, err := bmcresolver.FindBMCsForSecret(ctx, r.Client, bmcSecret.Name)
	if err != nil {
		logger.Error(err, "Failed to find BMCs during cleanup")
		// Continue with deletion
		controllerutil.RemoveFinalizer(bmcSecret, bmcSecretFinalizer)
		if err := r.Update(ctx, bmcSecret); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Delete secrets from backend
	for _, bmc := range bmcs {
		region := bmcresolver.ExtractRegionFromBMC(&bmc, regionLabelKey)
		hostname := bmcresolver.GetHostnameFromBMC(&bmc)

		path, err := pathBuilder.Build(secretbackend.PathVariables{
			Region:   region,
			Hostname: hostname,
			Username: username,
		})
		if err != nil {
			logger.Error(err, "Failed to build path during cleanup", "bmc", bmc.Name)
			continue
		}

		if err := backend.DeleteSecret(ctx, path); err != nil {
			logger.Error(err, "Failed to delete secret from backend", "path", path)
			// Continue with other deletions
			continue
		}

		logger.Info("Deleted secret from backend", "path", path)
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(bmcSecret, bmcSecretFinalizer)
	if err := r.Update(ctx, bmcSecret); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// needsUpdate checks if the secret needs to be updated in the backend
func (r *BMCSecretReconciler) needsUpdate(ctx context.Context, backend secretbackend.Backend, path, password string) (bool, error) {
	// Check if secret exists
	exists, err := backend.SecretExists(ctx, path)
	if err != nil {
		return false, err
	}

	if !exists {
		return true, nil
	}

	// Read current secret
	currentData, err := backend.ReadSecret(ctx, path)
	if err != nil {
		return false, err
	}

	// Compare passwords
	currentPassword, ok := currentData["password"].(string)
	if !ok {
		return true, nil
	}

	return currentPassword != password, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *BMCSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Get sync label configuration to create predicate
	syncLabel, err := r.BackendFactory.GetSyncLabel(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get sync label configuration: %w", err)
	}

	// Create predicate for label filtering
	var labelPredicate predicate.Predicate
	if syncLabel != "" {
		labelPredicate = predicate.NewPredicateFuncs(func(object client.Object) bool {
			if object.GetLabels() == nil {
				return false
			}
			_, exists := object.GetLabels()[syncLabel]
			return exists
		})
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCSecret{})

	// Apply label predicate if sync label is configured
	if labelPredicate != nil {
		builder = builder.WithEventFilter(labelPredicate)
	}

	return builder.
		Watches(
			&metalv1alpha1.BMC{},
			handler.EnqueueRequestsFromMapFunc(r.findBMCSecretsForBMC),
		).
		Complete(r)
}

// findBMCSecretsForBMC finds BMCSecrets that should be reconciled when a BMC changes
func (r *BMCSecretReconciler) findBMCSecretsForBMC(ctx context.Context, obj client.Object) []reconcile.Request {
	bmc := obj.(*metalv1alpha1.BMC)
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: bmc.Spec.BMCSecretRef.Name}},
	}
}

// updateSyncStatus creates or updates the BMCSecretSyncStatus resource
func (r *BMCSecretReconciler) updateSyncStatus(ctx context.Context, bmcSecretName string, backendPaths []configv1alpha1.BackendPath, totalPaths, successfulPaths, failedPaths int) error {
	logger := log.FromContext(ctx)

	syncStatusName := fmt.Sprintf("%s-sync-status", bmcSecretName)
	syncStatus := &configv1alpha1.BMCSecretSyncStatus{}

	// Try to get existing status
	err := r.Get(ctx, types.NamespacedName{Name: syncStatusName}, syncStatus)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// Create new status resource
		syncStatus = &configv1alpha1.BMCSecretSyncStatus{
			ObjectMeta: metav1.ObjectMeta{
				Name: syncStatusName,
			},
			Spec: configv1alpha1.BMCSecretSyncStatusSpec{
				BMCSecretRef: bmcSecretName,
			},
		}

		if err := r.Create(ctx, syncStatus); err != nil {
			logger.Error(err, "Failed to create BMCSecretSyncStatus")
			return err
		}

		// Fetch the created resource to update status
		if err := r.Get(ctx, types.NamespacedName{Name: syncStatusName}, syncStatus); err != nil {
			return err
		}
	}

	// Update status
	syncStatus.Status.BackendPaths = backendPaths
	syncStatus.Status.LastSyncAttempt = metav1.Now()
	syncStatus.Status.TotalPaths = totalPaths
	syncStatus.Status.SuccessfulPaths = successfulPaths
	syncStatus.Status.FailedPaths = failedPaths

	// Update conditions
	var condition metav1.Condition
	if failedPaths == 0 {
		condition = metav1.Condition{
			Type:               "Synced",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: syncStatus.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "AllPathsSynced",
			Message:            fmt.Sprintf("Successfully synced to %d backend paths", successfulPaths),
		}
	} else if successfulPaths > 0 {
		condition = metav1.Condition{
			Type:               "Synced",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: syncStatus.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "PartialSync",
			Message:            fmt.Sprintf("Synced %d/%d paths, %d failed", successfulPaths, totalPaths, failedPaths),
		}
	} else {
		condition = metav1.Condition{
			Type:               "Synced",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: syncStatus.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "SyncFailed",
			Message:            fmt.Sprintf("Failed to sync to %d paths", failedPaths),
		}
	}

	// Update or add condition
	setCondition(&syncStatus.Status.Conditions, condition)

	if err := r.Status().Update(ctx, syncStatus); err != nil {
		logger.Error(err, "Failed to update BMCSecretSyncStatus status")
		return err
	}

	return nil
}

// setCondition updates or adds a condition to the conditions slice
func setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	if conditions == nil {
		*conditions = []metav1.Condition{}
	}

	for i, condition := range *conditions {
		if condition.Type == newCondition.Type {
			// Update existing condition if status changed
			if condition.Status != newCondition.Status || condition.Reason != newCondition.Reason {
				(*conditions)[i] = newCondition
			}
			return
		}
	}

	// Add new condition
	*conditions = append(*conditions, newCondition)
}
