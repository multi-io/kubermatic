/*
Copyright 2020 The Kubermatic Kubernetes Platform contributors.

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

package machine

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/go-test/deep"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	openstack "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack/types"
	vsphere "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vsphere/types"
	mdutil "github.com/kubermatic/machine-controller/pkg/controller/machinedeployment/util"
	providerconfig "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestGetOpenstackProviderSpec(t *testing.T) {
	emptyCluster := &kubermaticv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Labels: map[string]string{
				"system-project": "bar",
			},
		},
		Spec: kubermaticv1.ClusterSpec{
			Cloud: kubermaticv1.CloudSpec{
				Openstack: &kubermaticv1.OpenstackCloudSpec{},
			},
		},
	}
	emptyNodeSpec := apiv1.NodeSpec{
		Cloud: apiv1.NodeCloudSpec{
			Openstack: &apiv1.OpenstackNodeSpec{},
		},
	}

	emptyDC := &kubermaticv1.Datacenter{
		Spec: kubermaticv1.DatacenterSpec{
			Openstack: &kubermaticv1.DatacenterSpecOpenstack{},
		},
	}
	for _, testcase := range []struct {
		name     string
		cluster  *kubermaticv1.Cluster
		nodeSpec apiv1.NodeSpec
		dc       *kubermaticv1.Datacenter
		result   *openstack.RawConfig
	}{
		{
			name:     "returns empty fields result",
			cluster:  emptyCluster,
			nodeSpec: emptyNodeSpec,
			dc:       emptyDC,
			result: &openstack.RawConfig{
				Tags: map[string]string{
					"metakube-cluster": "foo",
					"system-cluster":   "foo",
				},
			},
		},
		{
			name: "sets NodeSpec values",
			cluster: &kubermaticv1.Cluster{
				ObjectMeta: emptyCluster.ObjectMeta,
				Spec: kubermaticv1.ClusterSpec{
					Cloud: kubermaticv1.CloudSpec{
						Openstack: &kubermaticv1.OpenstackCloudSpec{
							FloatingIPPool: "the-IP-pool",
						},
					},
				},
			},
			nodeSpec: apiv1.NodeSpec{
				Cloud: apiv1.NodeCloudSpec{
					Openstack: &apiv1.OpenstackNodeSpec{
						Image:                     "Ubuntu",
						Flavor:                    "medium",
						InstanceReadyCheckPeriod:  "5s",
						InstanceReadyCheckTimeout: "5m",
						RootDiskSizeGB:            pointer.Int(100),
						AvailabilityZone:          "the-availability-zone",
						UseFloatingIP:             true,
						Tags: map[string]string{
							"node-spec-key": "node-spec-value",
						},
					},
				},
			},
			dc: emptyDC,
			result: &openstack.RawConfig{
				Image:                     &providerconfigtypes.ConfigVarString{Value: "Ubuntu"},
				Flavor:                    &providerconfigtypes.ConfigVarString{Value: "medium"},
				InstanceReadyCheckPeriod:  &providerconfigtypes.ConfigVarString{Value: "5s"},
				InstanceReadyCheckTimeout: &providerconfigtypes.ConfigVarString{Value: "5m"},
				FloatingIPPool:            &providerconfigtypes.ConfigVarString{Value: "the-IP-pool"},
				RootDiskSizeGB:            pointer.Int(100),
				AvailabilityZone:          &providerconfigtypes.ConfigVarString{Value: "the-availability-zone"},
				Tags: map[string]string{
					"metakube-cluster": "foo",
					"system-cluster":   "foo",
					"node-spec-key":    "node-spec-value",
				},
			},
		},
		{
			name: "sets DC fields",
			cluster: &kubermaticv1.Cluster{
				ObjectMeta: emptyCluster.ObjectMeta,
				Spec: kubermaticv1.ClusterSpec{
					Cloud: kubermaticv1.CloudSpec{
						Openstack: &kubermaticv1.OpenstackCloudSpec{
							FloatingIPPool: "the-IP-pool",
						},
					},
				},
			},
			nodeSpec: emptyNodeSpec,
			dc: &kubermaticv1.Datacenter{
				Spec: kubermaticv1.DatacenterSpec{
					Openstack: &kubermaticv1.DatacenterSpecOpenstack{
						AvailabilityZone:      "the-availability-zone",
						Region:                "the-region",
						AuthURL:               "https://somecloudcompany.com/auth",
						NodeVolumeAttachLimit: pointerUint(10),
						EnforceFloatingIP:     true,
						TrustDevicePath:       pointer.Bool(true),
					},
				},
			},
			result: &openstack.RawConfig{
				AvailabilityZone:      &providerconfigtypes.ConfigVarString{Value: "the-availability-zone"},
				Region:                &providerconfigtypes.ConfigVarString{Value: "the-region"},
				IdentityEndpoint:      &providerconfigtypes.ConfigVarString{Value: "https://somecloudcompany.com/auth"},
				NodeVolumeAttachLimit: pointerUint(10),
				FloatingIPPool:        &providerconfigtypes.ConfigVarString{Value: "the-IP-pool"},
				TrustDevicePath:       &providerconfigtypes.ConfigVarBool{Value: true},
				Tags: map[string]string{
					"metakube-cluster": "foo",
					"system-cluster":   "foo",
				},
			},
		},
		{
			name: "sets Cluster fields",
			cluster: &kubermaticv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"system-project":               "bar",
						kubermaticv1.ProjectIDLabelKey: "the project",
					},
				},
				Spec: kubermaticv1.ClusterSpec{
					Cloud: kubermaticv1.CloudSpec{
						Openstack: &kubermaticv1.OpenstackCloudSpec{
							Network:        "the network",
							SubnetID:       "the subnet id",
							ServerGroupID:  "the server group id",
							SecurityGroups: "the security group",
							FloatingIPPool: "the IP pool",
						},
					},
				},
			},
			dc:       emptyDC,
			nodeSpec: emptyNodeSpec,
			result: &openstack.RawConfig{
				Network:        &providerconfigtypes.ConfigVarString{Value: "the network"},
				Subnet:         &providerconfigtypes.ConfigVarString{Value: "the subnet id"},
				ServerGroupID:  &providerconfigtypes.ConfigVarString{Value: "the server group id"},
				SecurityGroups: []providerconfigtypes.ConfigVarString{{Value: "the security group"}},
				Tags: map[string]string{
					"metakube-cluster": "foo",
					"system-cluster":   "foo",
					"system-project":   "the project",
				},
			},
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			config := &openstack.RawConfig{}
			if result, err := getOpenstackProviderSpec(testcase.cluster, testcase.nodeSpec, testcase.dc); err != nil {
				t.Error(err)
			} else if err := json.Unmarshal(result.Raw, config); err != nil {
				t.Error(err)
			} else if diff := deep.Equal(testcase.result, config); diff != nil {
				t.Errorf("unexpected config: %v", diff)
			}
		})
	}
}

func pointerUint(n uint) *uint {
	return &n
}

func TestGetVSphereProviderSpec(t *testing.T) {
	tests := []struct {
		name        string
		cluster     *kubermaticv1.Cluster
		nodeSpec    apiv1.NodeSpec
		dc          *kubermaticv1.Datacenter
		wantRawConf vsphere.RawConfig
		wantErr     bool
	}{
		{
			name: "Datastore",
			nodeSpec: apiv1.NodeSpec{
				Cloud: apiv1.NodeCloudSpec{
					VSphere: &apiv1.VSphereNodeSpec{},
				},
			},
			cluster: &kubermaticv1.Cluster{
				Spec: kubermaticv1.ClusterSpec{
					Cloud: kubermaticv1.CloudSpec{
						VSphere: &kubermaticv1.VSphereCloudSpec{},
					},
				},
			},
			dc: &kubermaticv1.Datacenter{
				Spec: kubermaticv1.DatacenterSpec{
					VSphere: &kubermaticv1.DatacenterSpecVSphere{
						DefaultDatastore: "default-datastore",
					},
				},
			},
			wantRawConf: vsphere.RawConfig{
				Datastore: &providerconfigtypes.ConfigVarString{Value: "default-datastore"},
			},
		},
		{
			name: "Default datastore",
			nodeSpec: apiv1.NodeSpec{
				Cloud: apiv1.NodeCloudSpec{
					VSphere: &apiv1.VSphereNodeSpec{},
				},
			},
			cluster: &kubermaticv1.Cluster{
				Spec: kubermaticv1.ClusterSpec{
					Cloud: kubermaticv1.CloudSpec{
						VSphere: &kubermaticv1.VSphereCloudSpec{
							Datastore: "my-datastore",
						},
					},
				},
			},
			dc: &kubermaticv1.Datacenter{
				Spec: kubermaticv1.DatacenterSpec{
					VSphere: &kubermaticv1.DatacenterSpecVSphere{
						DefaultDatastore: "default-datastore",
					},
				},
			},
			wantRawConf: vsphere.RawConfig{
				Datastore: &providerconfigtypes.ConfigVarString{Value: "my-datastore"},
			},
		},
		{
			name: "Datastore cluster",
			nodeSpec: apiv1.NodeSpec{
				Cloud: apiv1.NodeCloudSpec{
					VSphere: &apiv1.VSphereNodeSpec{},
				},
			},
			cluster: &kubermaticv1.Cluster{
				Spec: kubermaticv1.ClusterSpec{
					Cloud: kubermaticv1.CloudSpec{
						VSphere: &kubermaticv1.VSphereCloudSpec{
							Datastore: "my-datastore-cluster",
						},
					},
				},
			},
			dc: &kubermaticv1.Datacenter{
				Spec: kubermaticv1.DatacenterSpec{
					VSphere: &kubermaticv1.DatacenterSpecVSphere{
						DefaultDatastore: "default-datastore",
					},
				},
			},
			wantRawConf: vsphere.RawConfig{
				Datastore: &providerconfigtypes.ConfigVarString{Value: "my-datastore-cluster"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getVSphereProviderSpec(tt.cluster, tt.nodeSpec, tt.dc)
			if (err != nil) != tt.wantErr {
				t.Errorf("getVSphereProviderSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotRawConf := vsphere.RawConfig{}
			err = json.Unmarshal(got.Raw, &gotRawConf)
			if err != nil {
				t.Fatalf("error occurred whil unmarshaling raw config: %v", err)
			}
			if !reflect.DeepEqual(gotRawConf, tt.wantRawConf) {
				t.Errorf("getVSphereProviderSpec() = %+v, want %+v", gotRawConf, tt.wantRawConf)
			}
		})
	}
}

func mdWithAndWithoutEmptyFields(t *testing.T) (*clusterv1alpha1.MachineDeployment, *clusterv1alpha1.MachineDeployment) {
	t.Helper()

	mdJSON := `{
    "apiVersion": "cluster.k8s.io/v1alpha1",
    "kind": "MachineDeployment",
    "metadata": {
        "annotations": {
            "machinedeployment.clusters.k8s.io/revision": "8"
        },
        "creationTimestamp": "2022-04-11T06:29:36Z",
        "finalizers": [
            "foregroundDeletion"
        ],
        "generation": 8,
        "name": "romantic-booth-worker-m8l2cl",
        "namespace": "kube-system",
        "resourceVersion": "32831",
        "uid": "b3918cb2-1f42-4e63-9286-1385071c0eda"
    },
    "spec": {
        "minReadySeconds": 0,
        "progressDeadlineSeconds": 600,
        "replicas": 3,
        "revisionHistoryLimit": 1,
        "selector": {
            "matchLabels": {
                "machine": "md-rm4kgghch9-tj2d9768n8"
            }
        },
        "strategy": {
            "rollingUpdate": {
                "maxSurge": 1,
                "maxUnavailable": 0
            },
            "type": "RollingUpdate"
        },
        "template": {
            "metadata": {
                "creationTimestamp": null,
                "labels": {
                    "machine": "md-rm4kgghch9-tj2d9768n8"
                }
            },
            "spec": {
                "metadata": {
                    "creationTimestamp": null,
                    "labels": {
                        "system/cluster": "rm4kgghch9",
                        "system/project": "lljd67jxt4"
                    }
                },
                "providerSpec": {
                    "value": {
                        "cloudProvider": "openstack",
                        "cloudProviderSpec": {
                            "username": "",
                            "password": "",
                            "availabilityZone": "dbl1",
                            "flavor": "m1.small",
                            "floatingIpPool": "ext-net",
                            "identityEndpoint": "https://api.dbl.cloud.syseleven.net:5000/v3",
                            "image": "Ubuntu Focal 20.04 (2022-03-25)",
                            "instanceReadyCheckPeriod": "5s",
                            "instanceReadyCheckTimeout": "120s",
                            "network": "metakube-rm4kgghch9",
                            "nodeVolumeAttachLimit": 25,
                            "region": "dbl",
                            "rootDiskSizeGB": null,
                            "securityGroups": [
                                "metakube-rm4kgghch9"
                            ],
                            "serverGroupID": "40b455b2-9549-4d5d-a66d-02b7b8f7e4dc",
                            "subnet": "ce0adb6f-a23f-43bd-84e0-e7030155eb59",
                            "tags": {
                                "metakube-cluster": "rm4kgghch9",
                                "system-cluster": "rm4kgghch9",
                                "system-project": "lljd67jxt4"
                            },
                            "trustDevicePath": null
                        },
                        "operatingSystem": "ubuntu",
                        "operatingSystemSpec": {
                            "distUpgradeOnBoot": false
                        },
                        "sshPublicKeys": []
                    }
                },
                "versions": {
                    "kubelet": "1.21.3"
                }
            }
        }
    },
    "status": {
        "availableReplicas": 3,
        "observedGeneration": 8,
        "readyReplicas": 3,
        "replicas": 3,
        "updatedReplicas": 3
    }
}
`
	mdEmptyFields := &clusterv1alpha1.MachineDeployment{}
	if err := json.Unmarshal([]byte(mdJSON), mdEmptyFields); err != nil {
		t.Fatalf("wrong machine deployment json: %v", err)
	}

	mdNoEmptyFields := mdEmptyFields.DeepCopy()

	providerSpec := &providerconfig.Config{}
	err := json.Unmarshal(mdNoEmptyFields.Spec.Template.Spec.ProviderSpec.Value.Raw, providerSpec)
	if err != nil {
		t.Fatal(err)
	}
	openstackConfig := &openstack.RawConfig{}
	err = json.Unmarshal(providerSpec.CloudProviderSpec.Raw, openstackConfig)
	if err != nil {
		t.Fatal(err)
	}
	openstackConfig.Username = nil
	openstackConfig.Password = nil
	providerSpec.CloudProviderSpec.Raw, err = json.Marshal(openstackConfig)
	if err != nil {
		t.Fatal(err)
	}
	mdNoEmptyFields.Spec.Template.Spec.ProviderSpec.Value.Raw, err = json.Marshal(providerSpec)
	if err != nil {
		t.Fatal(err)
	}

	openstackConfig.Username = &providerconfigtypes.ConfigVarString{}
	openstackConfig.Password = &providerconfigtypes.ConfigVarString{}
	providerSpec.CloudProviderSpec.Raw, err = json.Marshal(openstackConfig)
	if err != nil {
		t.Fatal(err)
	}
	mdEmptyFields.Spec.Template.Spec.ProviderSpec.Value.Raw, err = json.Marshal(providerSpec)
	if err != nil {
		t.Fatal(err)
	}

	return mdEmptyFields, mdNoEmptyFields
}

func changeFlavor(t *testing.T, md *clusterv1alpha1.MachineDeployment) *clusterv1alpha1.MachineDeployment {
	t.Helper()

	providerConfig := &providerconfig.Config{}
	openstackConfig := &openstack.RawConfig{}
	if err := json.Unmarshal(md.Spec.Template.Spec.ProviderSpec.Value.Raw, providerConfig); err != nil {
		t.Fatal(err)
	}
	err := json.Unmarshal(providerConfig.CloudProviderSpec.Raw, openstackConfig)
	if err != nil {
		t.Fatal(err)
	}

	openstackConfig.Flavor.Value = "changed flavor"
	providerConfig.CloudProviderSpec.Raw, err = json.Marshal(openstackConfig)
	if err != nil {
		t.Fatal(err)
	}
	md.Spec.Template.Spec.ProviderSpec.Value.Raw, err = json.Marshal(providerConfig)
	if err != nil {
		t.Fatal(err)
	}
	return md.DeepCopy()
}

func TestEnsureNoUnnecessaryOpenstackProviderSpecDiff(t *testing.T) {
	mdEmptyFields, mdNoEmptyFields := mdWithAndWithoutEmptyFields(t)
	*mdNoEmptyFields.Spec.Replicas++

	if a, b := mdutil.ComputeHash(mdEmptyFields.Spec.Template.DeepCopy()), mdutil.ComputeHash(mdNoEmptyFields.Spec.Template.DeepCopy()); a == b {
		t.Fatalf("hashes identical %v %v", a, b)
	}

	t.Log("testing preservation of empty fields")
	result, err := EnsureNoUnnecessaryOpenstackProviderSpecDiff(mdEmptyFields.DeepCopy(), mdNoEmptyFields.DeepCopy())
	if err != nil {
		t.Fatal(err)
	}

	if a, b := mdutil.ComputeHash(result.Spec.Template.DeepCopy()), mdutil.ComputeHash(mdEmptyFields.Spec.Template.DeepCopy()); a != b {
		t.Fatalf("unexpected machine-template-hash change: %d != %d", a, b)
	}

	t.Log("testing when rollover inevitable thus no need to preserve empty fields")
	noEmptyFieldsWithNewFlavor := changeFlavor(t, mdNoEmptyFields.DeepCopy())
	result, err = EnsureNoUnnecessaryOpenstackProviderSpecDiff(mdEmptyFields.DeepCopy(), noEmptyFieldsWithNewFlavor.DeepCopy())
	if err != nil {
		t.Fatal(err)
	}
	if a, b := mdutil.ComputeHash(result.Spec.Template.DeepCopy()), mdutil.ComputeHash(noEmptyFieldsWithNewFlavor.Spec.Template.DeepCopy()); a != b {
		t.Fatal("machine template without empty fields was not returned", a, b)
	}
}
