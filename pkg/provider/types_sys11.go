package provider

import (
	kubermaticv1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
)

// MachineDeploymentRequestProvider declares the set of methods for interacting with MachineDeploymentRequests
type MachineDeploymentRequestProvider interface {
	// New creates a new MachineDeploymentRequest for the given cluster and MachineDeployment
	New(userInfo *UserInfo, cluster *kubermaticv1.Cluster, mdrName string, md *v1alpha1.MachineDeployment) (*kubermaticv1.MachineDeploymentRequest, error)

	// List gets all MachineDeploymentRequests that belong to the given cluster
	// If you want to filter the result please take a look at ClusterListOptions
	List(userInfo *UserInfo, cluster *kubermaticv1.Cluster) ([]*kubermaticv1.MachineDeploymentRequest, error)

	// Get returns the given MachineDeploymentRequest
	Get(userInfo *UserInfo, cluster *kubermaticv1.Cluster, mdrName string) (*kubermaticv1.MachineDeploymentRequest, error)

	// Update updates a MachineDeploymentRequest
	Update(userInfo *UserInfo, cluster *kubermaticv1.Cluster, newMdRequest *kubermaticv1.MachineDeploymentRequest) (*kubermaticv1.MachineDeploymentRequest, error)

	// Delete deletes the given MachineDeploymentRequest
	Delete(userInfo *UserInfo, cluster *kubermaticv1.Cluster, mdrName string) error
}
