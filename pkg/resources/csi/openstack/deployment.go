package openstack

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"
)

var (
	defaultResourceRequirements = map[string]*corev1.ResourceRequirements{
		"cinder-csi-plugin": {
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("32Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
		"csi-attacher": {
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("32Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
		"csi-provisioner": {
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("48Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
		"csi-resizer": {
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("32Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
		"csi-snapshotter": {
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("32Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
		"liveness-probe": {
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("16Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
	}
)

// DeploymentCreator returns the function to create and update the nginx deployment
func DeploymentCreator(data *resources.TemplateData, priorityClasses bool) reconciling.NamedDeploymentCreatorGetter {
	return func() (string, reconciling.DeploymentCreator) {
		return resources.CsiCinderControllerPluginDeploymentName, func(dep *appsv1.Deployment) (*appsv1.Deployment, error) {

			dep.Name = resources.CsiCinderControllerPluginDeploymentName
			dep.Spec.Replicas = resources.Int32(1)
			dep.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: resources.ImagePullSecretName}}

			baseLabels := getBasePodLabels(data.Cluster())
			dep.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: baseLabels,
			}

			volumes := getVolumes()
			podLabels, err := data.GetPodTemplateLabels(resources.CsiCinderControllerPluginDeploymentName, volumes, baseLabels)
			if err != nil {
				return nil, fmt.Errorf("failed to create pod labels: %v", err)
			}

			if priorityClasses {
				dep.Spec.Template.Spec.PriorityClassName = resources.PriorityClassCsiCinderControllerPluginDeploymentName
			}

			dep.Spec.Template.ObjectMeta = metav1.ObjectMeta{
				Labels: podLabels,
			}

			driverVersion := getCsiDriverVersion(data.Cluster())

			dep.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:            "csi-attacher",
					Image:           data.ImageRegistry(resources.RegistryQuay) + "/k8scsi/csi-attacher:v3.1.0",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args: []string{
						"--csi-address=/var/lib/csi/sockets/pluginproxy/csi.sock",
						"--timeout=3m",
						"--leader-election",
						"--leader-election-namespace=kube-system",
						"--kubeconfig=/etc/kubernetes/kubeconfig/kubeconfig",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "socket-dir",
							MountPath: "/var/lib/csi/sockets/pluginproxy/",
						},
						{
							Name:      resources.CSICinderControllerKubeconfigSecretName,
							MountPath: "/etc/kubernetes/kubeconfig",
							ReadOnly:  true,
						},
					},
				},
				{
					Name:            "csi-provisioner",
					Image:           data.ImageRegistry(resources.RegistryQuay) + "/k8scsi/csi-provisioner:v2.1.0",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args: []string{
						"--csi-address=/var/lib/csi/sockets/pluginproxy/csi.sock",
						"--timeout=3m",
						"--leader-election",
						"--leader-election-namespace=kube-system",
						"--default-fstype=ext4",
						"--kubeconfig=/etc/kubernetes/kubeconfig/kubeconfig",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "socket-dir",
							MountPath: "/var/lib/csi/sockets/pluginproxy/",
						},
						{
							Name:      resources.CSICinderControllerKubeconfigSecretName,
							MountPath: "/etc/kubernetes/kubeconfig",
							ReadOnly:  true,
						},
					},
				},
				{
					Name:            "csi-snapshotter",
					Image:           data.ImageRegistry(resources.RegistryQuay) + "/k8scsi/csi-snapshotter:v2.1.4",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args: []string{
						"--csi-address=/var/lib/csi/sockets/pluginproxy/csi.sock",
						"--timeout=3m",
						"--leader-election",
						"--leader-election-namespace=kube-system",
						"--kubeconfig=/etc/kubernetes/kubeconfig/kubeconfig",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "socket-dir",
							MountPath: "/var/lib/csi/sockets/pluginproxy/",
						},
						{
							Name:      resources.CSICinderControllerKubeconfigSecretName,
							MountPath: "/etc/kubernetes/kubeconfig",
							ReadOnly:  true,
						},
					},
				},
				{
					Name:            "csi-resizer",
					Image:           data.ImageRegistry(resources.RegistryQuay) + "/k8scsi/csi-resizer:v1.1.0",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args: []string{
						"--csi-address=/var/lib/csi/sockets/pluginproxy/csi.sock",
						"--timeout=3m",
						"--leader-election",
						"--leader-election-namespace=kube-system",
						"--handle-volume-inuse-error=false",
						"--kubeconfig=/etc/kubernetes/kubeconfig/kubeconfig",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "socket-dir",
							MountPath: "/var/lib/csi/sockets/pluginproxy/",
						},
						{
							Name:      resources.CSICinderControllerKubeconfigSecretName,
							MountPath: "/etc/kubernetes/kubeconfig",
							ReadOnly:  true,
						},
					},
				},
				{
					Name:            "liveness-probe",
					Image:           data.ImageRegistry(resources.RegistryQuay) + "/k8scsi/livenessprobe:v2.2.0",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args: []string{
						"--csi-address=/var/lib/csi/sockets/pluginproxy/csi.sock",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "socket-dir",
							MountPath: "/var/lib/csi/sockets/pluginproxy/",
						},
						{
							Name:      resources.CSICinderControllerKubeconfigSecretName,
							MountPath: "/etc/kubernetes/kubeconfig",
							ReadOnly:  true,
						},
					},
				},
				{
					Name:            "cinder-csi-plugin",
					Image:           fmt.Sprintf("%s/n1k6e5t7/k8scloudprovider/cinder-csi-plugin:%s", data.ImageRegistry(resources.RegistryAwsEcr), driverVersion),
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command: []string{
						"/bin/cinder-csi-plugin",
						"--nodeid=requiredbutnotused",
						"--endpoint=unix://csi/csi.sock",
						"--cloud-config=/etc/config/config",
						fmt.Sprintf("--cluster=%s", data.Cluster().GetName()),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "socket-dir",
							MountPath: "/csi",
						},
						{
							Name:      resources.CloudConfigConfigMapName,
							MountPath: "/etc/config",
							ReadOnly:  true,
						},
					},
					Ports: []corev1.ContainerPort{
						{Name: "healthz", ContainerPort: 9808, Protocol: corev1.ProtocolTCP},
					},
					LivenessProbe: &corev1.Probe{
						Handler: corev1.Handler{
							HTTPGet: &corev1.HTTPGetAction{
								Path:   "/healthz",
								Port:   intstr.FromString("healthz"),
								Scheme: corev1.URISchemeHTTP,
							},
						},
						SuccessThreshold:    1,
						FailureThreshold:    5,
						InitialDelaySeconds: 10,
						TimeoutSeconds:      10,
						PeriodSeconds:       60,
					},
				},
			}

			err = resources.SetResourceRequirements(dep.Spec.Template.Spec.Containers, dep.Annotations, defaultResourceRequirements, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to set resource requirements: %v", err)
			}

			dep.Spec.Template.Spec.Volumes = volumes

			return dep, nil
		}
	}
}

func getVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "socket-dir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: resources.CloudConfigConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: resources.CloudConfigConfigMapName,
					},
				},
			},
		},
		{
			Name: resources.CABundleConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: resources.CABundleConfigMapName,
					},
				},
			},
		},
		{
			Name: resources.CSICinderControllerKubeconfigSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: resources.CSICinderControllerKubeconfigSecretName,
				},
			},
		},
	}
}

func getBasePodLabels(cluster *kubermaticv1.Cluster) map[string]string {
	additionalLabels := map[string]string{
		"cluster": cluster.Name,
	}
	return resources.BaseAppLabels(resources.CsiCinderControllerPluginDeploymentName, additionalLabels)
}

func getCsiDriverVersion(cluster *kubermaticv1.Cluster) string {
	switch cluster.Spec.Version.MajorMinor() {
	case "1.17", "1.18", "1.19":
		return "v1.19.0"
	case "1.20":
		return "v1.20.3"
	case "1.21":
		return "v1.21.0"
	case "1.22":
		return "v1.22.0"
	default:
		return fmt.Sprintf("v%s.0", cluster.Spec.Version.MajorMinor())
	}
}
