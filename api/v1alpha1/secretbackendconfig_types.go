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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretBackendConfigSpec defines the desired state of SecretBackendConfig
type SecretBackendConfigSpec struct {
	// Backend specifies the type of secret backend to use (vault, openbao)
	// +kubebuilder:validation:Enum=vault;openbao
	// +kubebuilder:validation:Required
	Backend string `json:"backend"`

	// VaultConfig contains Vault-specific configuration
	// +optional
	VaultConfig *VaultConfig `json:"vaultConfig,omitempty"`

	// OpenBaoConfig contains OpenBao-specific configuration
	// +optional
	OpenBaoConfig *OpenBaoConfig `json:"openBaoConfig,omitempty"`

	// PathTemplate is the template string for building secret paths
	// Available variables: {{.Region}}, {{.Hostname}}, {{.Username}}
	// +kubebuilder:default="bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
	// +optional
	PathTemplate string `json:"pathTemplate,omitempty"`

	// RegionLabelKey is the label key to extract region from BMC resources
	// +kubebuilder:default="region"
	// +optional
	RegionLabelKey string `json:"regionLabelKey,omitempty"`

	// SyncLabel is the label key that must be present on BMCSecrets to enable syncing
	// If not specified, all BMCSecrets will be synced
	// +optional
	SyncLabel string `json:"syncLabel,omitempty"`
}

// VaultConfig defines Vault-specific configuration
type VaultConfig struct {
	// Address is the Vault server URL
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// AuthMethod specifies the authentication method (kubernetes, token, approle)
	// +kubebuilder:validation:Enum=kubernetes;token;approle
	// +kubebuilder:default="kubernetes"
	// +optional
	AuthMethod string `json:"authMethod,omitempty"`

	// KubernetesAuth contains Kubernetes auth configuration
	// +optional
	KubernetesAuth *KubernetesAuthConfig `json:"kubernetesAuth,omitempty"`

	// TokenAuth contains token auth configuration
	// +optional
	TokenAuth *TokenAuthConfig `json:"tokenAuth,omitempty"`

	// MountPath is the KV secrets engine mount path
	// +kubebuilder:default="secret"
	// +optional
	MountPath string `json:"mountPath,omitempty"`

	// TLSConfig contains TLS configuration
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`

	// SecretEngines contains a list of secret engine configurations for different teams/purposes
	// Each entry can specify a different mount path, path template, and sync label
	// +optional
	SecretEngines []SecretEngineConfig `json:"secretEngines,omitempty"`
}

// SecretEngineConfig defines configuration for a specific secret engine/team
type SecretEngineConfig struct {
	// Name is a descriptive name for this secret engine configuration (e.g., "team-a", "prod-bmcs")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// MountPath is the KV secrets engine mount path for this configuration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`

	// PathTemplate is the template string for building secret paths
	// Available variables: {{.Region}}, {{.Hostname}}, {{.Username}}
	// +kubebuilder:default="bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
	// +optional
	PathTemplate string `json:"pathTemplate,omitempty"`

	// SyncLabel is the label key that must be present on BMCSecrets to sync to this engine
	// Format: key or key=value. If only key is specified, any value matches.
	// Example: "team=a" will match BMCSecrets with label team=a
	// Example: "sync-to-vault" will match BMCSecrets with any value for sync-to-vault label
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SyncLabel string `json:"syncLabel"`
}

// KubernetesAuthConfig defines Kubernetes authentication configuration
type KubernetesAuthConfig struct {
	// Role is the Vault role to authenticate as
	// +kubebuilder:validation:Required
	Role string `json:"role"`

	// Path is the Kubernetes auth mount path
	// +kubebuilder:default="kubernetes"
	// +optional
	Path string `json:"path,omitempty"`
}

// TokenAuthConfig defines token authentication configuration
type TokenAuthConfig struct {
	// SecretRef references a Kubernetes secret containing the Vault token
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`
}

// SecretReference defines a reference to a Kubernetes secret
type SecretReference struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Key is the key in the secret data
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// TLSConfig defines TLS configuration
type TLSConfig struct {
	// SkipVerify disables TLS certificate verification (not recommended for production)
	// +kubebuilder:default=false
	// +optional
	SkipVerify bool `json:"skipVerify,omitempty"`

	// CACert is the CA certificate for verifying the Vault server
	// +optional
	CACert string `json:"caCert,omitempty"`
}

// OpenBaoConfig defines OpenBao-specific configuration (future)
type OpenBaoConfig struct {
	// Address is the OpenBao server URL
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// Configuration fields will mirror VaultConfig
	// +optional
	AuthMethod string `json:"authMethod,omitempty"`
}

// SecretBackendConfigStatus defines the observed state of SecretBackendConfig.
type SecretBackendConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the SecretBackendConfig resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// SecretBackendConfig is the Schema for the secretbackendconfigs API
type SecretBackendConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SecretBackendConfig
	// +required
	Spec SecretBackendConfigSpec `json:"spec"`

	// status defines the observed state of SecretBackendConfig
	// +optional
	Status SecretBackendConfigStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SecretBackendConfigList contains a list of SecretBackendConfig
type SecretBackendConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SecretBackendConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretBackendConfig{}, &SecretBackendConfigList{})
}
