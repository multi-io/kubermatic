package prometheus

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
	vpaContainerPolicies = []autoscalingv1beta2.ContainerResourcePolicy{
		{
			ContainerName: resources.PrometheusStatefulSetName,
			MinAllowed: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("300Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			MaxAllowed: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("3Gi"),
				corev1.ResourceCPU:    resource.MustParse("600m"),
			},
		},
	}
)

// VPACreator returns a func to create/update the VPA
func VPACreator(ownerRef *metav1.OwnerReference, mode autoscalingv1beta2.UpdateMode) reconciling.NamedVerticalPodAutoscalerCreatorGetter {
	return resources.GetVerticalPodAutoscalerWithPolicies(resources.PrometheusStatefulSetName, ownerRef, mode, vpaContainerPolicies)
}
