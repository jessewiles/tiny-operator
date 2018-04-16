package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EchoServer refers to a simple python echo server
type EchoServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Type              string         `json:"type"`
	Spec              EchoServerSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EchoServerList refers to an array of EchoServer objects
type EchoServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EchoServer `json:"items"`
}

type EchoServerSpec struct {
	// Image is the docker tag of the echo server
	Image string `json:"image"`

	// Replicas is the number of EchoServer replicas k8s should maintain
	Replicas int32 `json:"replicas"`
}
