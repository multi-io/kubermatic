package common_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/handler/test"
	"k8c.io/kubermatic/v2/pkg/handler/test/hack"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
)

func TestGetNodeDeploymentRequest(t *testing.T) {
	t.Parallel()
	var replicas int32 = 1
	var paused = false
	var dymanicConfig = false
	testcases := []struct {
		Name                   string
		ProjectIDToSync        string
		ClusterIDToSync        string
		ExistingProject        *kubermaticv1.Project
		ExistingKubermaticUser *kubermaticv1.User
		ExistingAPIUser        *apiv1.User
		ExistingCluster        *kubermaticv1.Cluster
		ExistingMdrs           []*kubermaticv1.MachineDeploymentRequest
		NdrToGet               string
		ExistingKubermaticObjs []ctrlruntimeclient.Object
		ExpectedHTTPStatus     int
		ExpectedResponse       apiv1.NodeDeploymentRequest
	}{
		// scenario 1
		{
			Name:                   "scenario 1: get NDR for MDR for the given cluster",
			ClusterIDToSync:        test.GenDefaultCluster().Name,
			ProjectIDToSync:        test.GenDefaultProject().Name,
			ExistingKubermaticObjs: test.GenDefaultKubermaticObjects(test.GenTestSeed(), test.GenDefaultCluster()),
			ExistingAPIUser:        test.GenDefaultAPIUser(),
			ExistingMdrs: []*kubermaticv1.MachineDeploymentRequest{
				test.GenTestMachineDeploymentRequest(test.GenDefaultCluster(), "venus", `{"cloudProvider":"digitalocean","cloudProviderSpec":{"token":"dummy-token","region":"fra1","size":"2GB"}, "operatingSystem":"ubuntu", "operatingSystemSpec":{"distUpgradeOnBoot":true}}`, nil),
			},
			NdrToGet:           "venus",
			ExpectedHTTPStatus: http.StatusOK,
			ExpectedResponse: apiv1.NodeDeploymentRequest{
				ObjectMeta: apiv1.ObjectMeta{
					ID:   "venus",
					Name: "venus",
				},
				Spec: apiv1.NodeDeploymentRequestSpec{
					Nd: apiv1.NodeDeployment{
						ObjectMeta: apiv1.ObjectMeta{
							ID:   "venus",
							Name: "venus",
						},
						Spec: apiv1.NodeDeploymentSpec{
							Template: apiv1.NodeSpec{
								Cloud: apiv1.NodeCloudSpec{
									Digitalocean: &apiv1.DigitaloceanNodeSpec{
										Size: "2GB",
									},
								},
								OperatingSystem: apiv1.OperatingSystemSpec{
									Ubuntu: &apiv1.UbuntuSpec{
										DistUpgradeOnBoot: true,
									},
								},
								Versions: apiv1.NodeVersionInfo{
									Kubelet: "v9.9.9",
								},
							},
							Replicas:      replicas,
							Paused:        &paused,
							DynamicConfig: &dymanicConfig,
						},
						Status: clusterv1alpha1.MachineDeploymentStatus{},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/ndrequests/%s", tc.ProjectIDToSync, tc.ClusterIDToSync, tc.NdrToGet), strings.NewReader(""))
			res := httptest.NewRecorder()
			kubermaticObj := []ctrlruntimeclient.Object{}
			machineObj := []ctrlruntimeclient.Object{}
			kubernetesObj := []ctrlruntimeclient.Object{}
			kubermaticObj = append(kubermaticObj, tc.ExistingKubermaticObjs...)
			for _, existingMdr := range tc.ExistingMdrs {
				kubermaticObj = append(kubermaticObj, existingMdr)
			}
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.ExistingAPIUser, nil, kubernetesObj, machineObj, kubermaticObj, nil, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.ExpectedHTTPStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.ExpectedHTTPStatus, res.Code, res.Body.String())
			}

			if res.Code == http.StatusOK {
				bytes, err := json.Marshal(tc.ExpectedResponse)
				if err != nil {
					t.Fatalf("failed to marshall expected response %v", err)
				}

				test.CompareWithResult(t, res, string(bytes))
			}
		})
	}
}

func TestCreateNodeDeploymentRequest(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		Name                   string
		Body                   string
		ExpectedResponse       string
		ProjectID              string
		ClusterID              string
		HTTPStatus             int
		ExistingProject        *kubermaticv1.Project
		ExistingKubermaticUser *kubermaticv1.User
		ExistingAPIUser        *apiv1.User
		ExistingCluster        *kubermaticv1.Cluster
		ExistingKubermaticObjs []ctrlruntimeclient.Object
	}{
		// scenario 1
		{
			Name:                   "scenario 1: create a node deployment request that match the given spec",
			Body:                   `{"spec":{"nd":{"name":"mynodes","spec":{"replicas":1,"template":{"cloud":{"digitalocean":{"size":"s-1vcpu-1gb","backups":false,"ipv6":false,"monitoring":false,"tags":[]}},"operatingSystem":{"ubuntu":{"distUpgradeOnBoot":false}}}}}}}`,
			ExpectedResponse:       `{"id":"mynodes","name":"mynodes","creationTimestamp":"0001-01-01T00:00:00Z","spec":{"nd":{"id":"mynodes","name":"mynodes","creationTimestamp":"0001-01-01T00:00:00Z","spec":{"replicas":1,"template":{"cloud":{"digitalocean":{"size":"s-1vcpu-1gb","backups":false,"ipv6":false,"monitoring":false,"tags":["metakube","metakube-cluster-defClusterID","system-cluster-defClusterID","system-project-my-first-project-ID"]}},"operatingSystem":{"ubuntu":{"distUpgradeOnBoot":false}},"versions":{"kubelet":"9.9.9"},"labels":{"system/cluster":"defClusterID","system/project":"my-first-project-ID"}},"paused":false,"dynamicConfig":false},"status":{}}}}`,
			HTTPStatus:             http.StatusCreated,
			ProjectID:              test.GenDefaultProject().Name,
			ClusterID:              genNdrTestCluster().Name,
			ExistingKubermaticObjs: test.GenDefaultKubermaticObjects(test.GenTestSeed(), genNdrTestCluster()),
			ExistingAPIUser:        test.GenDefaultAPIUser(),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/ndrequests", tc.ProjectID, tc.ClusterID), strings.NewReader(tc.Body))
			res := httptest.NewRecorder()
			kubermaticObj := []ctrlruntimeclient.Object{}
			kubermaticObj = append(kubermaticObj, tc.ExistingKubermaticObjs...)
			ep, err := test.CreateTestEndpoint(*tc.ExistingAPIUser, []ctrlruntimeclient.Object{}, kubermaticObj, nil, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.HTTPStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.HTTPStatus, res.Code, res.Body.String())
			}

			nd := &apiv1.NodeDeployment{}
			err = json.Unmarshal(res.Body.Bytes(), nd)
			if err != nil {
				t.Fatal(err)
			}

			test.CompareWithResult(t, res, tc.ExpectedResponse)
		})
	}
}

func genNdrTestCluster() *kubermaticv1.Cluster {
	cluster := test.GenDefaultCluster()
	cluster.Spec.Cloud = kubermaticv1.CloudSpec{
		DatacenterName: "regular-do1",
	}
	return cluster
}
