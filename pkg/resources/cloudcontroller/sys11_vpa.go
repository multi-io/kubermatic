package cloudcontroller

import (
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"
)

var (
	// Limits are understood as limit only for recommendation (requests)
	// This means actual container limits = VPA limit * (limit to requests ratio)
	// This is always equal or higher than VPA limit
	osVPAContainerPolicies = []autoscalingv1beta2.ContainerResourcePolicy{
		{
			ContainerName: ccmContainerName,
			MinAllowed: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("25Mi"),
			},
			MaxAllowed: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
	}
	hetznerVPAContainerPolicies = []autoscalingv1beta2.ContainerResourcePolicy{
		{
			ContainerName: ccmContainerName,
			MinAllowed: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("20Mi"),
				corev1.ResourceCPU:    resource.MustParse("5m"),
			},
			MaxAllowed: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("50Mi"),
				corev1.ResourceCPU:    resource.MustParse("20m"),
			},
		},
	}

	vpaContainerPolicies = map[string][]autoscalingv1beta2.ContainerResourcePolicy{
		OpenstackCCMDeploymentName: osVPAContainerPolicies,
		HetznerCCMDeploymentName:   hetznerVPAContainerPolicies,
	}
)

// VPACreator returns a func to create/update the VPA
func VPACreator(data *resources.TemplateData, ownerRef *metav1.OwnerReference, mode autoscalingv1beta2.UpdateMode) reconciling.NamedVerticalPodAutoscalerCreatorGetter {
	var deploymentName string
	switch {
	case data.Cluster().Spec.Cloud.Openstack != nil:
		deploymentName = OpenstackCCMDeploymentName
	case data.Cluster().Spec.Cloud.Hetzner != nil:
		deploymentName = HetznerCCMDeploymentName
	}
	return resources.GetVerticalPodAutoscalerWithPolicies(deploymentName, ownerRef, mode, vpaContainerPolicies[deploymentName])
}
