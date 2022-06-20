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

package monitoring

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	autoscalingv1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/certificates"
	"k8c.io/kubermatic/v2/pkg/resources/kubestatemetrics"
	"k8c.io/kubermatic/v2/pkg/resources/prometheus"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"

	"k8s.io/apimachinery/pkg/api/resource"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) getClusterTemplateData(ctx context.Context, client ctrlruntimeclient.Client, cluster *kubermaticv1.Cluster) (*resources.TemplateData, error) {
	seed, err := r.seedGetter(ctx)
	if err != nil {
		return nil, err
	}

	datacenter, found := seed.Spec.Datacenters[cluster.Spec.Cloud.DatacenterName]
	if !found {
		return nil, fmt.Errorf("failed to get datacenter %s", cluster.Spec.Cloud.DatacenterName)
	}

	return resources.NewTemplateDataBuilder().
		WithContext(ctx).
		WithClient(client).
		WithCluster(cluster).
		WithDatacenter(&datacenter).
		WithSeed(seed.DeepCopy()).
		WithOverwriteRegistry(r.overwriteRegistry).
		WithNodePortRange(r.nodePortRange).
		WithNodeAccessNetwork(r.nodeAccessNetwork).
		WithEtcdDiskSize(resource.Quantity{}).
		WithMonitoringEnvironmentLabel(r.monitoringEnvironmentLabel).
		WithMonitoringScrapeAnnotationPrefix(r.monitoringScrapeAnnotationPrefix).
		WithInClusterPrometheusRulesFile(r.inClusterPrometheusRulesFile).
		WithInClusterPrometheusRemoteWriteConfigsFile(r.inClusterPrometheusRemoteWriteConfigsFile).
		WithInClusterPrometheusDefaultRulesDisabled(r.inClusterPrometheusDisableDefaultRules).
		WithInClusterPrometheusDefaultScrapingConfigsDisabled(r.inClusterPrometheusDisableDefaultScrapingConfigs).
		WithInClusterPrometheusScrapingConfigsFile(r.inClusterPrometheusScrapingConfigsFile).
		WithVersions(r.versions).
		// WithBackupPeriod(20 * time.Minute).
		Build(), nil
}

func (r *Reconciler) ensureRoles(ctx context.Context, cluster *kubermaticv1.Cluster) error {
	getters := []reconciling.NamedRoleCreatorGetter{
		prometheus.RoleCreator(),
	}

	return reconciling.ReconcileRoles(ctx, getters, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster)))
}

func (r *Reconciler) ensureRoleBindings(ctx context.Context, cluster *kubermaticv1.Cluster) error {
	getters := []reconciling.NamedRoleBindingCreatorGetter{
		prometheus.RoleBindingCreator(cluster.Status.NamespaceName),
	}

	return reconciling.ReconcileRoleBindings(ctx, getters, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster)))
}

// GetDeploymentCreators returns all DeploymentCreators that are currently in use
func GetDeploymentCreators(data *resources.TemplateData, priorityClasses bool) []reconciling.NamedDeploymentCreatorGetter {
	creators := []reconciling.NamedDeploymentCreatorGetter{
		kubestatemetrics.DeploymentCreator(data, priorityClasses),
	}

	return creators
}

func (r *Reconciler) ensureDeployments(ctx context.Context, cluster *kubermaticv1.Cluster, data *resources.TemplateData, priorityClasses bool) error {
	creators := GetDeploymentCreators(data, priorityClasses)

	return reconciling.ReconcileDeployments(ctx, creators, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster)))
}

// GetSecretCreatorOperations returns all SecretCreators that are currently in use
func GetSecretCreatorOperations(data *resources.TemplateData) []reconciling.NamedSecretCreatorGetter {
	return []reconciling.NamedSecretCreatorGetter{
		certificates.GetClientCertificateCreator(
			resources.PrometheusApiserverClientCertificateSecretName,
			resources.PrometheusCertUsername, nil,
			resources.PrometheusClientCertificateCertSecretKey,
			resources.PrometheusClientCertificateKeySecretKey,
			data.GetRootCA,
		),
	}
}

func (r *Reconciler) ensureSecrets(ctx context.Context, cluster *kubermaticv1.Cluster, data *resources.TemplateData) error {
	namedSecretCreatorGetters := GetSecretCreatorOperations(data)

	if err := reconciling.ReconcileSecrets(ctx, namedSecretCreatorGetters, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster))); err != nil {
		return fmt.Errorf("failed to ensure that the Secret exists: %v", err)
	}

	return nil
}

// GetConfigMapCreators returns all ConfigMapCreators that are currently in use
func GetConfigMapCreators(data *resources.TemplateData) []reconciling.NamedConfigMapCreatorGetter {
	return []reconciling.NamedConfigMapCreatorGetter{
		prometheus.ConfigMapCreator(data),
	}
}

func (r *Reconciler) ensureConfigMaps(ctx context.Context, cluster *kubermaticv1.Cluster, data *resources.TemplateData) error {
	creators := GetConfigMapCreators(data)

	if err := reconciling.ReconcileConfigMaps(ctx, creators, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster))); err != nil {
		return fmt.Errorf("failed to ensure that the ConfigMap exists: %v", err)
	}

	return nil
}

// GetStatefulSetCreators returns all StatefulSetCreators that are currently in use
func GetStatefulSetCreators(data *resources.TemplateData, priorityClasses bool) []reconciling.NamedStatefulSetCreatorGetter {
	return []reconciling.NamedStatefulSetCreatorGetter{
		prometheus.StatefulSetCreator(data, priorityClasses),
	}
}

func (r *Reconciler) ensureStatefulSets(ctx context.Context, cluster *kubermaticv1.Cluster, data *resources.TemplateData, priorityClasses bool) error {
	creators := GetStatefulSetCreators(data, priorityClasses)

	return reconciling.ReconcileStatefulSets(ctx, creators, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster)))
}

func (r *Reconciler) ensureVerticalPodAutoscalers(ctx context.Context, cluster *kubermaticv1.Cluster) error {
	deploymentNames := []string{
		resources.KubeStateMetricsDeploymentName,
	}
	statefulSetNames := []string{
		resources.PrometheusStatefulSetName,
	}

	creators, err := resources.GetVerticalPodAutoscalersForAll(
		ctx,
		r.Client,
		deploymentNames,
		statefulSetNames,
		cluster.Status.NamespaceName,
		r.features.VPA)
	if err != nil {
		return fmt.Errorf("failed to create the functions to handle VPA resources: %v", err)
	}
	return reconciling.ReconcileVerticalPodAutoscalers(ctx, creators, cluster.Status.NamespaceName, r.Client)
}

// Sys11: https://youtrack.syseleven.net/issue/K8s-5633
// Replace `ensureVerticalPodAutoscalers` function with our own.
// This uses each package's `CreateVPA` function instead of copying the resource quota as containerPolicies.
func (r *Reconciler) sys11EnsureVerticalPodAutoscalers(ctx context.Context, cluster *kubermaticv1.Cluster) error {
	namespace := cluster.Status.NamespaceName

	// K8s-5637: There's an exact copy of these two functions in `kubernetes` package.
	// Discussion: https://gitlab.syseleven.de/kubernetes/kubermatic/-/merge_requests/190#note_197206
	//FIXME: Extract as one function once generics are GA
	getDeploymentOwnerReference := func(name string) (*metav1.OwnerReference, error) {
		var obj appsv1.Deployment
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &obj); err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get owner reference of deployment '%s' for vpa: not found", name)
			}
			return nil, fmt.Errorf("failed to get owner reference of deployment '%s' for vpa: %v", name, err)
		}
		return metav1.NewControllerRef(&obj, obj.GroupVersionKind()), nil
	}
	getStatefulSetOwnerReference := func(name string) (*metav1.OwnerReference, error) {
		var obj appsv1.StatefulSet
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &obj); err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get owner reference of statefulset '%s' for vpa: not found", name)
			}
			return nil, fmt.Errorf("failed to get owner reference of statefulset '%s' for vpa: %v", name, err)
		}
		return metav1.NewControllerRef(&obj, obj.GroupVersionKind()), nil
	}

	updateMode := autoscalingv1beta2.UpdateModeOff
	if r.features.VPA {
		updateMode = autoscalingv1beta2.UpdateModeAuto
	}

	kubeStateMetricsOwnerRef, err := getDeploymentOwnerReference(resources.KubeStateMetricsDeploymentName)
	if err != nil {
		return err
	}
	prometheusOwnerRef, err := getStatefulSetOwnerReference(resources.PrometheusStatefulSetName)
	if err != nil {
		return err
	}

	creators := []reconciling.NamedVerticalPodAutoscalerCreatorGetter{
		kubestatemetrics.VPACreator(kubeStateMetricsOwnerRef, updateMode),
		prometheus.VPACreator(prometheusOwnerRef, updateMode),
	}

	return reconciling.ReconcileVerticalPodAutoscalers(ctx, creators, cluster.Status.NamespaceName, r.Client)
}

// Sys11: End

// GetServiceCreators returns all service creators that are currently in use
func GetServiceCreators(data *resources.TemplateData) []reconciling.NamedServiceCreatorGetter {
	return []reconciling.NamedServiceCreatorGetter{
		prometheus.ServiceCreator(data),
	}
}

func (r *Reconciler) ensureServices(ctx context.Context, cluster *kubermaticv1.Cluster, data *resources.TemplateData) error {
	creators := GetServiceCreators(data)

	return reconciling.ReconcileServices(ctx, creators, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster)))
}

// GetServiceCreators returns all service creators that are currently in use
func GetServiceAccountCreators() []reconciling.NamedServiceAccountCreatorGetter {
	return []reconciling.NamedServiceAccountCreatorGetter{
		prometheus.ServiceAccountCreator(),
	}
}

func (r *Reconciler) ensureServiceAccounts(ctx context.Context, cluster *kubermaticv1.Cluster, data *resources.TemplateData) error {
	creators := GetServiceAccountCreators()

	return reconciling.ReconcileServiceAccounts(ctx, creators, cluster.Status.NamespaceName, r.Client, reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster)))
}
