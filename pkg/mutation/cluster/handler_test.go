package cluster

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"text/template"

	admissionv1 "k8s.io/api/admission/v1"

	logrtesting "github.com/go-logr/logr/testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
)

var (
	testScheme = runtime.NewScheme()
)

func init() {
	_ = kubermaticv1.AddToScheme(testScheme)
}

func TestNuberOfFields(t *testing.T) {
	settings := kubermaticv1.ComponentSettings{}
	t.Log("If this test is failing, you probably added or deleted field from kubermaticv1.ComponentSettings struct, and did not update mutation handler yet.")
	if want, got := 5, reflect.ValueOf(settings).NumField(); want != got {
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
}

func TestHandle(t *testing.T) {
	handler := AdmissionHandler{
		log:     &logrtesting.NullLogger{},
		decoder: newDecoder(t),
		client: fake.NewClientBuilder().WithObjects(
			&kubermaticv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: kubermaticv1.ClusterSpec{},
			}).Build(),
		settings: kubermaticv1.ComponentSettings{
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
				Raw: rawClusterGen{Name: "foo", Namespace: "default"}.Do(),
			},
		},
	})

	if len(res.Patches) != 2 {
		t.Fatal("expected patches")
	}

	if p := res.Patches[0]; p.Operation != "remove" || p.Path != fieldJSONPath {
		t.Fatal("first patch is not delete patch")
	}

	expect := handler.settings
	expect.Etcd.Replicas = pointer.Int32Ptr(10)
	if p := res.Patches[1]; p.Operation != "add" || p.Path != fieldJSONPath || !reflect.DeepEqual(p.Value, expect) {
		t.Fatalf("second patch is not add patch, got %+v", p.Value)
	}
}

func newDecoder(t *testing.T) *admission.Decoder {
	t.Helper()
	d, err := admission.NewDecoder(testScheme)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

type rawClusterGen struct {
	Name      string
	Namespace string
}

func (r rawClusterGen) Do() []byte {
	tmpl, _ := template.New("cluster").Parse(`{
  "apiVersion": "kubermatic.k8s.io/v1",
  "kind": "Cluster",
  "metadata": {
	"name": "{{ .Name }}",
	"namespace": "{{ .Namespace}}"
  },
  "spec": {
  }
}`)
	sb := bytes.Buffer{}
	_ = tmpl.Execute(&sb, r)
	return sb.Bytes()
}
