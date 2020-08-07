// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "k8c.io/kubermatic/v2/pkg/crd/client/clientset/versioned/typed/kubermatic/v1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeKubermaticV1 struct {
	*testing.Fake
}

func (c *FakeKubermaticV1) Addons(namespace string) v1.AddonInterface {
	return &FakeAddons{c, namespace}
}

func (c *FakeKubermaticV1) AddonConfigs() v1.AddonConfigInterface {
	return &FakeAddonConfigs{c}
}

func (c *FakeKubermaticV1) Clusters() v1.ClusterInterface {
	return &FakeClusters{c}
}

func (c *FakeKubermaticV1) ConstraintTemplates() v1.ConstraintTemplateInterface {
	return &FakeConstraintTemplates{c}
}

func (c *FakeKubermaticV1) EtcdBackups(namespace string) v1.EtcdBackupInterface {
	return &FakeEtcdBackups{c, namespace}
}

func (c *FakeKubermaticV1) ExternalClusters() v1.ExternalClusterInterface {
	return &FakeExternalClusters{c}
}

func (c *FakeKubermaticV1) KubermaticSettings() v1.KubermaticSettingInterface {
	return &FakeKubermaticSettings{c}
}

func (c *FakeKubermaticV1) Projects() v1.ProjectInterface {
	return &FakeProjects{c}
}

func (c *FakeKubermaticV1) Users() v1.UserInterface {
	return &FakeUsers{c}
}

func (c *FakeKubermaticV1) UserProjectBindings() v1.UserProjectBindingInterface {
	return &FakeUserProjectBindings{c}
}

func (c *FakeKubermaticV1) UserSSHKeys() v1.UserSSHKeyInterface {
	return &FakeUserSSHKeys{c}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeKubermaticV1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
