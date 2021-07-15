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
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"gomodules.xyz/jsonpatch/v2"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AdmissionHandler for mutating Kubermatic Cluster CRD.
type AdmissionHandler struct {
	log     logr.Logger
	decoder *admission.Decoder

	defaultSettings kubermaticv1.ComponentSettings
}

// NewAdmissionHandler returns a new cluster mutation AdmissionHandler.
func NewAdmissionHandler(s kubermaticv1.ComponentSettings) *AdmissionHandler {
	return &AdmissionHandler{defaultSettings: s}
}

func (h *AdmissionHandler) InjectLogger(l logr.Logger) error {
	h.log = l.WithName("cluster-mutation-handler")
	return nil
}

func (h *AdmissionHandler) InjectDecoder(d *admission.Decoder) error {
	h.decoder = d
	return nil
}

//nolint:golint,cyclomatic
func (h *AdmissionHandler) Handle(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
	cluster := &kubermaticv1.Cluster{}
	oldCluster := &kubermaticv1.Cluster{}

	switch req.Operation {
	case admissionv1.Create:
		if err := h.decoder.Decode(req, cluster); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		settings := cluster.Spec.ComponentsOverride

		if settings.Apiserver.Replicas == nil {
			settings.Apiserver.Replicas = h.defaultSettings.Apiserver.Replicas
		}
		if settings.Apiserver.Resources == nil {
			settings.Apiserver.Resources = h.defaultSettings.Apiserver.Resources
		}
		if settings.Apiserver.EndpointReconcilingDisabled == nil {
			settings.Apiserver.EndpointReconcilingDisabled = h.defaultSettings.Apiserver.EndpointReconcilingDisabled
		}
		if settings.Apiserver.NodePortRange == "" {
			settings.Apiserver.NodePortRange = h.defaultSettings.Apiserver.NodePortRange
		}
		if settings.ControllerManager.Replicas == nil {
			settings.ControllerManager.Replicas = h.defaultSettings.ControllerManager.Replicas
		}
		if settings.ControllerManager.Resources == nil {
			settings.ControllerManager.Resources = h.defaultSettings.ControllerManager.Resources
		}
		if settings.ControllerManager.Tolerations == nil {
			settings.ControllerManager.Tolerations = h.defaultSettings.ControllerManager.Tolerations
		}
		if settings.ControllerManager.LeaseDurationSeconds == nil {
			settings.ControllerManager.LeaseDurationSeconds = h.defaultSettings.ControllerManager.LeaseDurationSeconds
		}
		if settings.ControllerManager.RenewDeadlineSeconds == nil {
			settings.ControllerManager.RenewDeadlineSeconds = h.defaultSettings.ControllerManager.RenewDeadlineSeconds
		}
		if settings.ControllerManager.RetryPeriodSeconds == nil {
			settings.ControllerManager.RetryPeriodSeconds = h.defaultSettings.ControllerManager.RetryPeriodSeconds
		}
		if settings.Scheduler.Replicas == nil {
			settings.Scheduler.Replicas = h.defaultSettings.Scheduler.Replicas
		}
		if settings.Scheduler.Resources == nil {
			settings.Scheduler.Resources = h.defaultSettings.Scheduler.Resources
		}
		if settings.Scheduler.Tolerations == nil {
			settings.Scheduler.Tolerations = h.defaultSettings.Scheduler.Tolerations
		}
		if settings.Scheduler.LeaseDurationSeconds == nil {
			settings.Scheduler.LeaseDurationSeconds = h.defaultSettings.Scheduler.LeaseDurationSeconds
		}
		if settings.Scheduler.RenewDeadlineSeconds == nil {
			settings.Scheduler.RenewDeadlineSeconds = h.defaultSettings.Scheduler.RenewDeadlineSeconds
		}
		if settings.Scheduler.RetryPeriodSeconds == nil {
			settings.Scheduler.RetryPeriodSeconds = h.defaultSettings.Scheduler.RetryPeriodSeconds
		}
		if settings.Etcd.Replicas == nil {
			settings.Etcd.Replicas = h.defaultSettings.Etcd.Replicas
		}
		if settings.Etcd.Resources == nil {
			settings.Etcd.Resources = h.defaultSettings.Etcd.Resources
		}
		if settings.Etcd.Tolerations == nil {
			settings.Etcd.Tolerations = h.defaultSettings.Etcd.Tolerations
		}
		if settings.Etcd.DiskSize == nil {
			settings.Etcd.DiskSize = h.defaultSettings.Etcd.DiskSize
		}
		if settings.Etcd.StorageClass == "" {
			settings.Etcd.StorageClass = h.defaultSettings.Etcd.StorageClass
		}
		if settings.Prometheus.Resources == nil {
			settings.Prometheus.Resources = h.defaultSettings.Prometheus.Resources
		}
		if !settings.Openstack.EnableIngressHostname {
			settings.Openstack.EnableIngressHostname = h.defaultSettings.Openstack.EnableIngressHostname
		}

		response := webhook.Patched("default componentsOverride",
			jsonpatch.Operation{
				Operation: "remove",
				Path:      "/spec/componentsOverride",
			},
			jsonpatch.Operation{
				Operation: "add",
				Path:      "/spec/componentsOverride",
				Value:     settings,
			})
		response.UID = req.UID
		return response
	case admissionv1.Update:
		if err := h.decoder.Decode(req, cluster); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decoder.DecodeRaw(req.OldObject, oldCluster); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if err := h.mutateUpdate(ctx, oldCluster, cluster); err != nil {
			h.log.Info("cluster mutation failed", "error", err)
			return webhook.Errored(http.StatusInternalServerError, fmt.Errorf("cluster mutation request %s failed: %v", req.UID, err))
		}

		mutatedCluster, err := json.Marshal(cluster)
		if err != nil {
			return webhook.Errored(http.StatusInternalServerError, fmt.Errorf("marshaling cluster object failed: %v", err))
		}

		return admission.PatchResponseFromRaw(req.Object.Raw, mutatedCluster)
	case admissionv1.Delete:
		// NOP
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s not supported on cluster resources", req.Operation))
	}
	return webhook.Allowed(fmt.Sprintf("no mutation done for request %s", req.UID))
}

func (h *AdmissionHandler) mutateUpdate(ctx context.Context, oldCluster, newCluster *kubermaticv1.Cluster) error {
	// This part of the code handles the CCM/CSI migration. It currently works
	// only for OpenStack clusters, in the following way:
	//   * Add the CCM/CSI migration annotations
	//   * Enable the UseOctaiva flag
	switch {
	case newCluster.Spec.Cloud.Openstack != nil:
		if v, oldV := newCluster.Spec.Features[kubermaticv1.ClusterFeatureExternalCloudProvider],
			oldCluster.Spec.Features[kubermaticv1.ClusterFeatureExternalCloudProvider]; v && !oldV {
			if newCluster.ObjectMeta.Annotations == nil {
				newCluster.ObjectMeta.Annotations = map[string]string{}
			}

			newCluster.ObjectMeta.Annotations[kubermaticv1.CCMMigrationNeededAnnotation] = ""
			newCluster.ObjectMeta.Annotations[kubermaticv1.CSIMigrationNeededAnnotation] = ""
			newCluster.Spec.Cloud.Openstack.UseOctavia = pointer.BoolPtr(true)
		}
	}

	return nil
}

func (h *AdmissionHandler) SetupWebhookWithManager(mgr ctrlruntime.Manager) {
	mgr.GetWebhookServer().Register("/mutate-kubermatic-k8s-io-cluster", &webhook.Admission{Handler: h})
}
