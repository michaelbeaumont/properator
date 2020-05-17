package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ref tells us which version of our repo to track
type Ref struct {
	// +optional
	Branch string `json:"branch,omitempty"`
	// +optional
	Sha string `json:"sha,omitempty"`
	// +optional
	Tag string `json:"tag,omitempty"`
	// +optional
	PullRequest int `json:"pullRequest,omitempty"`
}

// Repo defines the Github repo
type Repo struct {
	Owner string `json:"owner,omitempty"`
	Name  string `json:"name,omitempty"`
}

// RefReleaseSpec defines the desired state of RefRelease
type RefReleaseSpec struct {
	// Repo refers to a github repository
	Repo Repo `json:"repo,omitempty"`
	// Repo refers to either a branch, tag or commit along with a pull request
	// number
	Ref Ref `json:"ref,omitempty"`
}

// RefReleaseStatus defines the observed state of RefRelease
type RefReleaseStatus struct {
	// Deployment status determines the deployment URL
	DeploymentURL string `json:"deploymentURL,omitempty"`
}

// +kubebuilder:object:root=true

// RefRelease is the Schema for the refreleases API
type RefRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RefReleaseSpec   `json:"spec,omitempty"`
	Status RefReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RefReleaseList contains a list of RefRelease
type RefReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RefRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RefRelease{}, &RefReleaseList{})
}
