/*
Copyright 2026 Paperclip Inc.

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

// SelfConfigAction represents an action category that can be allowed for self-configuration.
// +kubebuilder:validation:Enum=skills;config;workspaceFiles;envVars
type SelfConfigAction string

const (
	SelfConfigActionSkills         SelfConfigAction = "skills"
	SelfConfigActionConfig         SelfConfigAction = "config"
	SelfConfigActionWorkspaceFiles SelfConfigAction = "workspaceFiles"
	SelfConfigActionEnvVars        SelfConfigAction = "envVars"
)

// SelfConfigPhase represents the processing state of a self-config request.
type SelfConfigPhase string

const (
	SelfConfigPhasePending SelfConfigPhase = "Pending"
	SelfConfigPhaseApplied SelfConfigPhase = "Applied"
	SelfConfigPhaseFailed  SelfConfigPhase = "Failed"
	SelfConfigPhaseDenied  SelfConfigPhase = "Denied"
)

// SelfConfigureSpec configures whether an agent can modify its own instance.
type SelfConfigureSpec struct {
	// Enabled enables self-configuration for this instance.
	// When true, the agent can create OpenClawSelfConfig resources to modify its own spec.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// AllowedActions restricts which action categories the agent can perform.
	// If empty and enabled is true, no actions are allowed (fail-safe).
	// +kubebuilder:validation:MaxItems=4
	// +optional
	AllowedActions []SelfConfigAction `json:"allowedActions,omitempty"`
}

// OpenClawSelfConfigSpec defines the desired changes to an OpenClawInstance.
type OpenClawSelfConfigSpec struct {
	// InstanceRef is the name of the parent OpenClawInstance in the same namespace.
	// +kubebuilder:validation:MinLength=1
	InstanceRef string `json:"instanceRef"`

	// AddSkills is a list of skills to add to the instance.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	AddSkills []string `json:"addSkills,omitempty"`

	// RemoveSkills is a list of skills to remove from the instance.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	RemoveSkills []string `json:"removeSkills,omitempty"`

	// ConfigPatch is a partial JSON configuration to deep-merge into the instance config.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ConfigPatch *RawConfig `json:"configPatch,omitempty"`

	// AddWorkspaceFiles maps filenames to content to add to the workspace.
	// +kubebuilder:validation:MaxProperties=10
	// +optional
	AddWorkspaceFiles map[string]string `json:"addWorkspaceFiles,omitempty"`

	// RemoveWorkspaceFiles is a list of workspace filenames to remove.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	RemoveWorkspaceFiles []string `json:"removeWorkspaceFiles,omitempty"`

	// AddEnvVars is a list of environment variables to add (plain values only).
	// +kubebuilder:validation:MaxItems=10
	// +optional
	AddEnvVars []SelfConfigEnvVar `json:"addEnvVars,omitempty"`

	// RemoveEnvVars is a list of environment variable names to remove.
	// +kubebuilder:validation:MaxItems=10
	// +optional
	RemoveEnvVars []string `json:"removeEnvVars,omitempty"`
}

// SelfConfigEnvVar defines a plain-value environment variable (no secret refs).
type SelfConfigEnvVar struct {
	// Name of the environment variable.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value of the environment variable.
	Value string `json:"value"`
}

// OpenClawSelfConfigStatus defines the observed state of OpenClawSelfConfig.
type OpenClawSelfConfigStatus struct {
	// Phase is the processing state of this request.
	// +kubebuilder:validation:Enum=Pending;Applied;Failed;Denied
	// +optional
	Phase SelfConfigPhase `json:"phase,omitempty"`

	// Message provides human-readable details about the current phase.
	// +optional
	Message string `json:"message,omitempty"`

	// CompletionTime is when the request reached a terminal phase.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ocsc
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpenClawSelfConfig is the Schema for the openclawselfconfigs API.
// It represents a request from an agent to modify its own OpenClawInstance spec.
type OpenClawSelfConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenClawSelfConfigSpec   `json:"spec,omitempty"`
	Status OpenClawSelfConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenClawSelfConfigList contains a list of OpenClawSelfConfig
type OpenClawSelfConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenClawSelfConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenClawSelfConfig{}, &OpenClawSelfConfigList{})
}
