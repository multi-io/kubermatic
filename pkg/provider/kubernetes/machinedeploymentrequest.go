package kubernetes

import (
	"context"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"k8c.io/kubermatic/v2/pkg/provider"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MachineDeploymentRequestProvider struct that holds required components of the MachineDeploymentRequestProvider implementation
type MachineDeploymentRequestProvider struct {
	// createSeedImpersonatedClient is used as a ground for impersonation
	// whenever a connection to Seed API server is required
	createSeedImpersonatedClient ImpersonationClient
}

// NewMachineDeploymentRequestProvider returns a new MachineDeploymentRequest provider that respects RBAC policies
// it uses createSeedImpersonatedClient to create a connection that uses user impersonation
func NewMachineDeploymentRequestProvider(
	createSeedImpersonatedClient ImpersonationClient) *MachineDeploymentRequestProvider {
	return &MachineDeploymentRequestProvider{
		createSeedImpersonatedClient: createSeedImpersonatedClient,
	}
}

// New creates a new MachineDeploymentRequest for the given cluster
func (p *MachineDeploymentRequestProvider) New(userInfo *provider.UserInfo, cluster *kubermaticv1.Cluster, mdrName string, md *v1alpha1.MachineDeployment) (*kubermaticv1.MachineDeploymentRequest, error) {
	seedImpersonatedClient, err := createImpersonationClientWrapperFromUserInfo(userInfo, p.createSeedImpersonatedClient)
	if err != nil {
		return nil, err
	}

	gv := kubermaticv1.SchemeGroupVersion

	mdr := &kubermaticv1.MachineDeploymentRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mdrName,
			Namespace:       cluster.Status.NamespaceName,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(cluster, gv.WithKind("Cluster"))},
			Labels:          map[string]string{},
		},
		Spec: kubermaticv1.MachineDeploymentRequestSpec{
			Md: *md,
		},
	}

	if err = seedImpersonatedClient.Create(context.Background(), mdr); err != nil {
		return nil, err
	}

	return mdr, nil
}

// Get returns the given MachineDeploymentRequest
func (p *MachineDeploymentRequestProvider) Get(userInfo *provider.UserInfo, cluster *kubermaticv1.Cluster, mdrName string) (*kubermaticv1.MachineDeploymentRequest, error) {
	seedImpersonatedClient, err := createImpersonationClientWrapperFromUserInfo(userInfo, p.createSeedImpersonatedClient)
	if err != nil {
		return nil, err
	}

	mdr := &kubermaticv1.MachineDeploymentRequest{}
	if err := seedImpersonatedClient.Get(context.Background(), ctrlruntimeclient.ObjectKey{Namespace: cluster.Status.NamespaceName, Name: mdrName}, mdr); err != nil {
		return nil, err
	}
	return mdr, nil
}

// List returns all MachineDeploymentRequests in the given cluster
func (p *MachineDeploymentRequestProvider) List(userInfo *provider.UserInfo, cluster *kubermaticv1.Cluster) ([]*kubermaticv1.MachineDeploymentRequest, error) {
	seedImpersonatedClient, err := createImpersonationClientWrapperFromUserInfo(userInfo, p.createSeedImpersonatedClient)
	if err != nil {
		return nil, err
	}

	mdrList := &kubermaticv1.MachineDeploymentRequestList{}
	if err := seedImpersonatedClient.List(context.Background(), mdrList, ctrlruntimeclient.InNamespace(cluster.Status.NamespaceName)); err != nil {
		return nil, err
	}

	result := []*kubermaticv1.MachineDeploymentRequest{}
	for _, mdr := range mdrList.Items {
		result = append(result, mdr.DeepCopy())
	}

	return result, nil
}

// Update updates an MachineDeploymentRequest
func (p *MachineDeploymentRequestProvider) Update(userInfo *provider.UserInfo, cluster *kubermaticv1.Cluster, newMdRequest *kubermaticv1.MachineDeploymentRequest) (*kubermaticv1.MachineDeploymentRequest, error) {
	seedImpersonatedClient, err := createImpersonationClientWrapperFromUserInfo(userInfo, p.createSeedImpersonatedClient)
	if err != nil {
		return nil, err
	}

	newMdRequest.Namespace = cluster.Status.NamespaceName
	if err := seedImpersonatedClient.Update(context.Background(), newMdRequest); err != nil {
		return nil, err
	}

	return newMdRequest, nil
}

// Delete deletes the given MachineDeploymentRequest
func (p *MachineDeploymentRequestProvider) Delete(userInfo *provider.UserInfo, cluster *kubermaticv1.Cluster, mdrName string) error {
	seedImpersonatedClient, err := createImpersonationClientWrapperFromUserInfo(userInfo, p.createSeedImpersonatedClient)
	if err != nil {
		return err
	}

	return seedImpersonatedClient.Delete(context.Background(), &kubermaticv1.MachineDeploymentRequest{ObjectMeta: metav1.ObjectMeta{Name: mdrName, Namespace: cluster.Status.NamespaceName}})
}

func MachineDeploymentRequestProviderFactory(mapper meta.RESTMapper, seedKubeconfigGetter provider.SeedKubeconfigGetter) provider.MachineDeploymentRequestProviderGetter {
	return func(seed *kubermaticv1.Seed) (provider.MachineDeploymentRequestProvider, error) {
		cfg, err := seedKubeconfigGetter(context.TODO(), seed)
		if err != nil {
			return nil, err
		}
		defaultImpersonationClientForSeed := NewImpersonationClient(cfg, mapper)

		return NewMachineDeploymentRequestProvider(defaultImpersonationClientForSeed.CreateImpersonatedClient), nil
	}
}
