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

package mutation

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"text/template"

	"k8s.io/utils/pointer"

	logrtesting "github.com/go-logr/logr/testing"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	testScheme = runtime.NewScheme()
)

func init() {
	_ = kubermaticv1.AddToScheme(testScheme)
}

//nolint:goconst
func TestHandle(t *testing.T) {
	tests := []struct {
		name            string
		req             webhook.AdmissionRequest
		wantAllowed     bool
		wantAnnotations bool
		wantUseOctavia  bool
	}{
		{
			name: "Create cluster success",
			req: webhook.AdmissionRequest{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					RequestKind: &metav1.GroupVersionKind{
						Group:   kubermaticv1.GroupName,
						Version: kubermaticv1.GroupVersion,
						Kind:    "Cluster",
					},
					Name: "foo",
					Object: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack", ExternalCloudProvider: true}.Do(),
					},
				},
			},
			wantAllowed: true,
		},
		{
			name: "Delete cluster success",
			req: webhook.AdmissionRequest{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					RequestKind: &metav1.GroupVersionKind{
						Group:   kubermaticv1.GroupName,
						Version: kubermaticv1.GroupVersion,
						Kind:    "Cluster",
					},
					Name: "foo",
					Object: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack", ExternalCloudProvider: true}.Do(),
					},
				},
			},
			wantAllowed: true,
		},
		{
			name: "Update OpenStack cluster to enable the CCM/CSI migration",
			req: webhook.AdmissionRequest{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					RequestKind: &metav1.GroupVersionKind{
						Group:   kubermaticv1.GroupName,
						Version: kubermaticv1.GroupVersion,
						Kind:    "Cluster",
					},
					Name: "foo",
					Object: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack", ExternalCloudProvider: true}.Do(),
					},
					OldObject: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack", ExternalCloudProvider: false}.Do(),
					},
				},
			},
			wantAllowed:     true,
			wantAnnotations: true,
			wantUseOctavia:  true,
		},
		{
			name: "Update OpenStack cluster with enabled CCM/CSI migration",
			req: webhook.AdmissionRequest{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					RequestKind: &metav1.GroupVersionKind{
						Group:   kubermaticv1.GroupName,
						Version: kubermaticv1.GroupVersion,
						Kind:    "Cluster",
					},
					Name: "foo",
					Object: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack", ExternalCloudProvider: true}.Do(),
					},
					OldObject: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack", ExternalCloudProvider: true}.Do(),
					},
				},
			},
			wantAllowed:     true,
			wantAnnotations: false,
			wantUseOctavia:  false,
		},
		{
			name: "Update non-OpenStack cluster to enable CCM/CSI migration",
			req: webhook.AdmissionRequest{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					RequestKind: &metav1.GroupVersionKind{
						Group:   kubermaticv1.GroupName,
						Version: kubermaticv1.GroupVersion,
						Kind:    "Cluster",
					},
					Name: "foo",
					Object: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "hetzner", ExternalCloudProvider: true}.Do(),
					},
					OldObject: runtime.RawExtension{
						Raw: rawClusterGen{Name: "foo", CloudProvider: "hetzner", ExternalCloudProvider: false}.Do(),
					},
				},
			},
			wantAllowed:     true,
			wantAnnotations: false,
			wantUseOctavia:  false,
		},
	}
	for _, tt := range tests {
		d, err := admission.NewDecoder(testScheme)
		if err != nil {
			t.Fatalf("error occurred while creating decoder: %v", err)
		}
		handler := AdmissionHandler{
			log:     &logrtesting.NullLogger{},
			decoder: d,
		}
		t.Run(tt.name, func(t *testing.T) {
			res := handler.Handle(context.TODO(), tt.req)
			if res.Allowed != tt.wantAllowed {
				t.Logf("Response: %v", res)
				t.Fatalf("Allowed %t, but wanted %t", res.Allowed, tt.wantAllowed)
			}

			foundPatchCCMAnnotation := false
			foundPatchCSIAnnotation := false
			foundPatchUseOctavia := false

			for _, patch := range res.Patches {
				switch {
				case patch.Operation == "add" && patch.Path == "/metadata/annotations" && patch.Value != nil:
					if m, ok := patch.Value.(map[string]interface{}); ok {
						for k := range m {
							if k == kubermaticv1.CCMMigrationNeededAnnotation {
								foundPatchCCMAnnotation = true
							}
							if k == kubermaticv1.CSIMigrationNeededAnnotation {
								foundPatchCSIAnnotation = true
							}
						}
					}
				case patch.Operation == "add" && patch.Path == "/spec/cloud/openstack/useOctavia":
					if v, ok := patch.Value.(bool); ok {
						if v {
							foundPatchUseOctavia = true
						}
					}
				}
			}

			if tt.wantAnnotations != foundPatchCCMAnnotation {
				t.Errorf("ccm-migration.k8c.io/migration-needed: expected: %v, found: %v", tt.wantAnnotations, foundPatchCCMAnnotation)
			}
			if tt.wantAnnotations != foundPatchCSIAnnotation {
				t.Errorf("csi-migration.k8c.io/migration-needed: expected: %v, found: %v", tt.wantAnnotations, foundPatchCSIAnnotation)
			}
			if tt.wantUseOctavia != foundPatchUseOctavia {
				t.Errorf(".spec.Cloud.openstack.UseOctavia: expected: %v, found: %v", tt.wantAnnotations, foundPatchUseOctavia)
			}
		})
	}
}

func TestHandleCreateMutation(t *testing.T) {
	d, err := admission.NewDecoder(testScheme)
	if err != nil {
		t.Fatalf("error occurred while creating decoder: %v", err)
	}
	handler := AdmissionHandler{
		log:     &logrtesting.NullLogger{},
		decoder: d,
		defaultSettings: kubermaticv1.ComponentSettings{
			Etcd: kubermaticv1.EtcdStatefulSetSettings{
				DeploymentSettings: kubermaticv1.DeploymentSettings{
					Replicas: pointer.Int32Ptr(10),
				},
			},
		},
	}

	res := handler.Handle(context.Background(), webhook.AdmissionRequest{
		AdmissionRequest: admissionv1.AdmissionRequest{
			RequestKind: &metav1.GroupVersionKind{
				Group:   kubermaticv1.GroupName,
				Version: kubermaticv1.GroupVersion,
				Kind:    "Cluster",
			},
			Operation: admissionv1.Create,
			Object: runtime.RawExtension{
				Raw: rawClusterGen{Name: "foo", CloudProvider: "openstack"}.Do(),
			},
		},
	})

	if len(res.Patches) != 2 {
		t.Fatal("expected patches")
	}

	if p := res.Patches[0]; p.Operation != "remove" || p.Path != "/spec/componentsOverride" {
		t.Fatal("first patch is not delete patch")
	}

	expect := handler.defaultSettings
	expect.Etcd.Replicas = pointer.Int32Ptr(10)
	if p := res.Patches[1]; p.Operation != "add" || p.Path != "/spec/componentsOverride" || !reflect.DeepEqual(p.Value, expect) {
		t.Fatalf("second patch is not add patch, got %+v", p.Value)
	}
}

type rawClusterGen struct {
	Name                  string
	CloudProvider         string
	ExternalCloudProvider bool
}

func (r rawClusterGen) Do() []byte {
	tmpl, _ := template.New("cluster").Parse(`
	{
	"apiVersion": "kubermatic.k8s.io/v1",
	"kind": "Cluster",
	"metadata": {
	  "name": "{{ .Name }}"
	},
	"spec": {
	  "features": {
		"externalCloudProvider": {{ .ExternalCloudProvider }}
	  },
      "cloud": {
        "{{ .CloudProvider }}": {}
      }
	}
}`)
	sb := bytes.Buffer{}
	_ = tmpl.Execute(&sb, r)
	return sb.Bytes()
}

func TestNumberOfFields(t *testing.T) {
	settings := kubermaticv1.ComponentSettings{}
	t.Log("If this test is failing, you probably added or deleted field from kubermaticv1.ComponentSettings struct, and did not update mutation handler yet.")
	if want, got := 6, reflect.ValueOf(settings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.NumField=%d, got %d", want, got)
	}

	if want, got := 3, reflect.ValueOf(settings.Apiserver).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Apiserver.NumField=%d, got %d", want, got)
	}
	if want, got := 3, reflect.ValueOf(settings.Apiserver.DeploymentSettings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Apiserver.DeploymentSettings.NumField=%d, got %d", want, got)
	}

	if want, got := 2, reflect.ValueOf(settings.ControllerManager).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.ControllerManager.NumField=%d, got %d", want, got)
	}

	if want, got := 3, reflect.ValueOf(settings.ControllerManager.DeploymentSettings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.ControllerManager.DeploymentSettings.NumField=%d, got %d", want, got)
	}

	if want, got := 3, reflect.ValueOf(settings.ControllerManager.LeaderElectionSettings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.ControllerManager.LeaderElectionSettings.NumField=%d, got %d", want, got)
	}

	if want, got := 2, reflect.ValueOf(settings.Scheduler).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Scheduler.NumField=%d, got %d", want, got)
	}

	if want, got := 3, reflect.ValueOf(settings.Scheduler.DeploymentSettings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Scheduler.DeploymentSettings.NumField=%d, got %d", want, got)
	}

	if want, got := 3, reflect.ValueOf(settings.Scheduler.LeaderElectionSettings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Scheduler.LeaderElectionSettings.NumField=%d, got %d", want, got)
	}

	if want, got := 3, reflect.ValueOf(settings.Etcd).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Etcd.NumField=%d, got %d", want, got)
	}
	if want, got := 3, reflect.ValueOf(settings.Etcd.DeploymentSettings).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Etcd.DeploymentSettings.NumField=%d, got %d", want, got)
	}
	if want, got := 1, reflect.ValueOf(settings.Prometheus).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Prometheus.NumField=%d, got %d", want, got)
	}
	if want, got := 1, reflect.ValueOf(settings.Openstack).NumField(); want != got {
		t.Fatalf("want kubermaticv1.ComponentSettings{}.Openstack.NumField=%d, got %d", want, got)
	}
}
