package clusterproxy

import (
	"github.com/kubermatic/kubermatic/pkg/resources"
	"github.com/kubermatic/kubermatic/pkg/resources/reconciling"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceCreator returns the function to reconcile the service
func ServiceCreator(data *resources.TemplateData) reconciling.NamedServiceCreatorGetter {
	return func() (string, reconciling.ServiceCreator) {
		return name, func(se *corev1.Service) (*corev1.Service, error) {
			se.Name = name
			se.OwnerReferences = []metav1.OwnerReference{data.GetClusterRef()}
			se.Labels = resources.BaseAppLabels(name, nil)

			se.Spec.Selector = map[string]string{
				resources.AppLabelKey: name,
				"cluster":             data.Cluster().Name,
			}
			se.Spec.Ports = []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(nginxPort),
				},
				{
					Name:       "linkerdhttp",
					Port:       linkerdNginxPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(linkerdNginxPort),
				},
			}

			return se, nil
		}
	}
}
