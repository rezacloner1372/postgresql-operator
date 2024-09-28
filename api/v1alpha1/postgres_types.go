// Define Go structs representing the Postgres CR.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PostgresSpec struct {
	Version     string      `json:"version"`
	Persistence Persistence `json:"persistence"`
	Auth        Auth        `json:"auth"`
}

type Persistence struct {
	Size string `json:"size"`
}

type Auth struct {
	Databse   string `json:"database"`
	SecretRef string `json:"secretRef"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type PostgresStatus struct {
	Ready bool `json:"ready"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Postgres struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresSpec   `json:"spec,omitempty"`
	Status PostgresStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PostgresList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Postgres `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Postgres{}, &PostgresList{})
}
