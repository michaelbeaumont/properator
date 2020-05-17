package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployment tells us about our deployment
type Deployment struct {
	Status DeploymentStatus `json:"statuses,omitempty"`
	// Current Sha
	Sha string `json:"sha,omitempty"`
	// Owner
	Owner string `json:"owner,omitempty"`
	// Name
	Name string `json:"name,omitempty"`
	// Ref
	Ref string `json:"ref,omitempty"`
	// ID
	ID int64 `json:"id,omitempty"`
}

// DeploymentStatus tells us about a deployment for some Sha
type DeploymentStatus struct {
	// URL determines the deployment URL
	URL string `json:"url,omitempty"`
	// State determines the deployment state
	State string `json:"state,omitempty"`
}

// +kubebuilder:object:root=true

// GithubDeployment is the Schema for the githubdeployment API
type GithubDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Deployment       `json:"spec,omitempty"`
	Status DeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GithubDeploymentList contains a list of GithubDeployment
type GithubDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GithubDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GithubDeployment{}, &GithubDeploymentList{})
}
