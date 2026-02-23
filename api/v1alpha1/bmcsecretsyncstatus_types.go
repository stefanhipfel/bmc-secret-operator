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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSecretSyncStatusSpec defines the desired state of BMCSecretSyncStatus
type BMCSecretSyncStatusSpec struct {
	// BMCSecretRef references the BMCSecret being tracked
	BMCSecretRef string `json:"bmcSecretRef"`
}

// BackendPath represents a single backend path that was synced
type BackendPath struct {
	// Path is the full path in the backend where the secret is stored
	Path string `json:"path"`

	// BMCName is the name of the BMC resource associated with this path
	BMCName string `json:"bmcName"`

	// Region is the region extracted from the BMC
	Region string `json:"region"`

	// Hostname is the hostname extracted from the BMC
	Hostname string `json:"hostname"`

	// Username is the username from the BMCSecret
	Username string `json:"username"`

	// LastSyncTime is the timestamp when this path was last synced
	LastSyncTime metav1.Time `json:"lastSyncTime"`

	// SyncStatus indicates if the sync was successful
	// +kubebuilder:validation:Enum=Success;Failed
	SyncStatus string `json:"syncStatus"`

	// ErrorMessage contains the error if sync failed
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// BMCSecretSyncStatusStatus defines the observed state of BMCSecretSyncStatus
type BMCSecretSyncStatusStatus struct {
	// BackendPaths lists all backend paths where this secret has been synced
	// +optional
	BackendPaths []BackendPath `json:"backendPaths,omitempty"`

	// LastSyncAttempt is the timestamp of the last sync attempt
	// +optional
	LastSyncAttempt metav1.Time `json:"lastSyncAttempt,omitempty"`

	// TotalPaths is the total number of paths that should be synced
	TotalPaths int `json:"totalPaths"`

	// SuccessfulPaths is the number of paths successfully synced
	SuccessfulPaths int `json:"successfulPaths"`

	// FailedPaths is the number of paths that failed to sync
	FailedPaths int `json:"failedPaths"`

	// Conditions represent the latest available observations of the sync status
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCSecret",type=string,JSONPath=`.spec.bmcSecretRef`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalPaths`
// +kubebuilder:printcolumn:name="Successful",type=integer,JSONPath=`.status.successfulPaths`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedPaths`
// +kubebuilder:printcolumn:name="Last Sync",type=date,JSONPath=`.status.lastSyncAttempt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BMCSecretSyncStatus is the Schema for the bmcsecretsyncstatuses API
type BMCSecretSyncStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSecretSyncStatusSpec   `json:"spec,omitempty"`
	Status BMCSecretSyncStatusStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCSecretSyncStatusList contains a list of BMCSecretSyncStatus
type BMCSecretSyncStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCSecretSyncStatus `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCSecretSyncStatus{}, &BMCSecretSyncStatusList{})
}
