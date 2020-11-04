package v1

import (
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// MachineDeploymentRequestResourceName represents "Resource" defined in Kubernetes
	MachineDeploymentRequestResourceName = "machinedeploymentrequests"

	// MachineDeploymentRequestKindName represents "Kind" defined in Kubernetes
	MachineDeploymentRequestKindName = "MachineDeploymentRequest"
)

//+genclient

// MachineDeploymentRequest specifies a request to create a MachineDeployment in a cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MachineDeploymentRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MachineDeploymentRequestSpec `json:"spec"`
}

// MachineDeploymentRequestSpec specifies details of a MachineDeploymentRequest
type MachineDeploymentRequestSpec struct {
	Md v1alpha1.MachineDeployment `json:"machinedeployment"`
}

// MachineDeploymentRequestList is a list of MachineDeploymentRequests
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MachineDeploymentRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MachineDeploymentRequest `json:"items"`
}
