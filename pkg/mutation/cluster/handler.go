package cluster

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"gomodules.xyz/jsonpatch/v2"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// ClusterDefaultComponentSettingsPath is a path to register webhook at.
	ClusterDefaultComponentSettingsPath = "/mutate-kubermatic-k8s-io-cluster"

	fieldJSONPath = "/spec/componentsOverride"
)

// AdmissionHandler for Kubermatic Cluster CRD.
type AdmissionHandler struct {
	log      logr.Logger
	decoder  *admission.Decoder
	client   client.Client
	settings kubermaticv1.ComponentSettings
}

// NewAdmissionHandler returns a new cluster.AdmissionHandler.
func NewAdmissionHandler(client client.Client, defaults kubermaticv1.ComponentSettings) *AdmissionHandler {
	return &AdmissionHandler{
		client:   client,
		settings: defaults,
	}
}

func (h *AdmissionHandler) Handle(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
	cluster := &kubermaticv1.Cluster{}

	if req.Operation != admissionv1beta1.Create {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s not supported on cluster resources", req.Operation))
	}

	if err := h.decoder.Decode(req, cluster); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	settings := cluster.Spec.ComponentsOverride

	if settings.Apiserver.Replicas == nil {
		settings.Apiserver.Replicas = h.settings.Apiserver.Replicas
	}
	if settings.Apiserver.Resources == nil {
		settings.Apiserver.Resources = h.settings.Apiserver.Resources
	}
	if settings.Apiserver.EndpointReconcilingDisabled == nil {
		settings.Apiserver.EndpointReconcilingDisabled = h.settings.Apiserver.EndpointReconcilingDisabled
	}
	if settings.Apiserver.NodePortRange == "" {
		settings.Apiserver.NodePortRange = h.settings.Apiserver.NodePortRange
	}
	if settings.ControllerManager.Replicas == nil {
		settings.ControllerManager.Replicas = h.settings.ControllerManager.Replicas
	}
	if settings.ControllerManager.Resources == nil {
		settings.ControllerManager.Resources = h.settings.ControllerManager.Resources
	}
	if settings.ControllerManager.Tolerations == nil {
		settings.ControllerManager.Tolerations = h.settings.ControllerManager.Tolerations
	}
	if settings.Scheduler.Replicas == nil {
		settings.Scheduler.Replicas = h.settings.Scheduler.Replicas
	}
	if settings.Scheduler.Resources == nil {
		settings.Scheduler.Resources = h.settings.Scheduler.Resources
	}
	if settings.Scheduler.Tolerations == nil {
		settings.Scheduler.Tolerations = h.settings.Scheduler.Tolerations
	}
	if settings.Etcd.Replicas == nil {
		settings.Etcd.Replicas = h.settings.Etcd.Replicas
	}
	if settings.Etcd.Resources == nil {
		settings.Etcd.Resources = h.settings.Etcd.Resources
	}
	if settings.Etcd.Tolerations == nil {
		settings.Etcd.Tolerations = h.settings.Etcd.Tolerations
	}
	if settings.Etcd.DiskSize == nil {
		settings.Etcd.DiskSize = h.settings.Etcd.DiskSize
	}
	if settings.Etcd.StorageClass == "" {
		settings.Etcd.StorageClass = h.settings.Etcd.StorageClass
	}
	if settings.Prometheus.Resources == nil {
		settings.Prometheus.Resources = h.settings.Prometheus.Resources
	}

	return webhook.Patched("default componentsOverride",
		jsonpatch.Operation{
			Operation: "remove",
			Path:      fieldJSONPath,
		},
		jsonpatch.Operation{
			Operation: "add",
			Path:      fieldJSONPath,
			Value:     settings,
		})
}

func (h *AdmissionHandler) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(ClusterDefaultComponentSettingsPath, &webhook.Admission{Handler: h})
}
