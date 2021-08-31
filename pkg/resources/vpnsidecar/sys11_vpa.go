package vpnsidecar

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	autoscalingv1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"
)

const OpenVPNClientName = "openvpn-client"
const DnatControllerName = "dnat-controller"

// Limits are understood as limit only for recommendation (requests)
// This means actual container limits = VPA limit * (limit to requests ratio)
// This is always equal or higher than VPA limit
var (
	DnatControllerVPAPolicy = autoscalingv1beta2.ContainerResourcePolicy{
		ContainerName: DnatControllerName,
		MinAllowed: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("10Mi"),
			corev1.ResourceCPU:    resource.MustParse("5m"),
		},
		MaxAllowed: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("50Mi"),
			corev1.ResourceCPU:    resource.MustParse("10m"),
		},
	}
	VpnClientVPAContainerPolicy = autoscalingv1beta2.ContainerResourcePolicy{
		ContainerName: OpenVPNClientName,
		MinAllowed: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("10Mi"),
			corev1.ResourceCPU:    resource.MustParse("5m"),
		},
		MaxAllowed: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("10Mi"),
			corev1.ResourceCPU:    resource.MustParse("20m"),
		},
	}
)
