package machine

import (
	"fmt"
	"testing"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/semver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeployment(t *testing.T) {
	for _, tc := range []struct {
		testname string
		c        *kubermaticv1.Cluster
		nd       *apiv1.NodeDeployment
		dc       *kubermaticv1.Datacenter
		keys     []*kubermaticv1.UserSSHKey
		data     resources.CredentialsData

		okResult func(*clusterv1alpha1.MachineDeployment) error
	}{
		{
			testname: "activate horizontal nodes autoscaler",
			c: &kubermaticv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-cluster",
				},
				Spec: kubermaticv1.ClusterSpec{
					Version: *semver.NewSemverOrDie("1.24.0"),
					Cloud: kubermaticv1.CloudSpec{
						Openstack: &kubermaticv1.OpenstackCloudSpec{},
					},
				},
			},
			nd: &apiv1.NodeDeployment{
				ObjectMeta: apiv1.ObjectMeta{
					Name: "foo-machines",
				},
				Spec: apiv1.NodeDeploymentSpec{
					MinReplicas: 1,
					MaxReplicas: 3,
					Template: apiv1.NodeSpec{
						Cloud: apiv1.NodeCloudSpec{
							Openstack: &apiv1.OpenstackNodeSpec{},
						},
						OperatingSystem: apiv1.OperatingSystemSpec{
							Ubuntu: &apiv1.UbuntuSpec{},
						},
					},
				},
			},

			dc: &kubermaticv1.Datacenter{
				Spec: kubermaticv1.DatacenterSpec{
					Openstack: &kubermaticv1.DatacenterSpecOpenstack{},
				},
			},

			okResult: func(md *clusterv1alpha1.MachineDeployment) error {
				if md.Annotations == nil {
					return fmt.Errorf("no annotations at all")
				}
				if md.Annotations[apiv1.AutoscalerMinSizeAnnotation] != "1" || md.Annotations[apiv1.AutoscalerMaxSizeAnnotation] != "3" {
					return fmt.Errorf("wrong annotations: %v", md.Annotations)
				}
				return nil
			},
		},
		{
			testname: "De-Activate horizontal nodes autoscaler",
			c: &kubermaticv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-cluster",
				},
				Spec: kubermaticv1.ClusterSpec{
					Version: *semver.NewSemverOrDie("1.24.0"),
					Cloud: kubermaticv1.CloudSpec{
						Openstack: &kubermaticv1.OpenstackCloudSpec{},
					},
				},
			},
			nd: &apiv1.NodeDeployment{
				ObjectMeta: apiv1.ObjectMeta{
					Annotations: map[string]string{
						apiv1.AutoscalerMinSizeAnnotation: "1",
						apiv1.AutoscalerMaxSizeAnnotation: "3",
					},
					Name: "foo-machines",
				},
				Spec: apiv1.NodeDeploymentSpec{
					Replicas: 3,
					Template: apiv1.NodeSpec{
						Cloud: apiv1.NodeCloudSpec{
							Openstack: &apiv1.OpenstackNodeSpec{},
						},
						OperatingSystem: apiv1.OperatingSystemSpec{
							Ubuntu: &apiv1.UbuntuSpec{},
						},
					},
				},
			},

			dc: &kubermaticv1.Datacenter{
				Spec: kubermaticv1.DatacenterSpec{
					Openstack: &kubermaticv1.DatacenterSpecOpenstack{},
				},
			},

			okResult: func(md *clusterv1alpha1.MachineDeployment) error {
				if md.Annotations == nil {
					return nil
				}
				if md.Annotations[apiv1.AutoscalerMinSizeAnnotation] != "" || md.Annotations[apiv1.AutoscalerMaxSizeAnnotation] != "" {
					return fmt.Errorf("wrong annotations: %v", md.Annotations)
				}
				return nil
			},
		},
	} {
		md, err := Deployment(tc.c, tc.nd, tc.dc, tc.keys, tc.data)
		if err != nil {
			t.Fatalf("build machine deployment: %v", err)
		}
		if err := tc.okResult(md); err != nil {
			t.Fatalf("%s: %v", tc.testname, err)
		}
	}
}
