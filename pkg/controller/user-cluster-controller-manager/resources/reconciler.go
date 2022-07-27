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

package resources

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net"

	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/cloudcontroller"
	cabundle "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/ca-bundle"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/cloudinitsettings"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/clusterautoscaler"
	controllermanager "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/controller-manager"
	coredns "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/core-dns"
	csimigration "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/csi-migration"
	csiopenstack "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/csi/openstack"
	dnatcontroller "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/dnat-controller"
	dnsautoscaler "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/dns-autoscaler"
	envoyagent "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/envoy-agent"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/konnectivity"
	kubestatemetrics "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/kube-state-metrics"
	machinecontroller "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/machine-controller"
	metricsserver "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/metrics-server"
	nodelocaldns "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/node-local-dns"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/openvpn"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/prometheus"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/scheduler"
	systembasicuser "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/system-basic-user"
	userauth "k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/user-auth"
	"k8c.io/kubermatic/v2/pkg/controller/user-cluster-controller-manager/resources/resources/usersshkeys"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/certificates/triple"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconcile creates, updates, or deletes Kubernetes resources to match the desired state.
func (r *reconciler) reconcile(ctx context.Context) error {
	caCert, err := r.caCert(ctx)
	if err != nil {
		return fmt.Errorf("failed to get caCert: %v", err)
	}
	userSSHKeys, err := r.userSSHKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to get userSSHKeys: %v", err)
	}
	cloudConfig, err := r.cloudConfig(ctx, resources.CloudConfigConfigMapName)
	if err != nil {
		return fmt.Errorf("failed to get cloudConfig: %v", err)
	}
	var CSICloudConfig []byte
	if r.cloudProvider == kubermaticv1.ProviderVSphere {
		CSICloudConfig, err = r.cloudConfig(ctx, resources.CSICloudConfigConfigMapName)
		if err != nil {
			return fmt.Errorf("failed to get cloudConfig: %v", err)
		}
	}

	data := reconcileData{
		caCert:         caCert,
		userSSHKeys:    userSSHKeys,
		cloudConfig:    cloudConfig,
		csiCloudConfig: CSICloudConfig,
		ccmMigration:   r.ccmMigration || r.ccmMigrationCompleted,
	}

	data.clusterAddress, data.ipFamily, data.k8sServiceAPIIP, _, err = r.networkingData(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster address: %w", err)
	}

	if !r.isKonnectivityEnabled {
		data.openVPNCACert, err = r.openVPNCA(ctx)
		if err != nil {
			return fmt.Errorf("failed to get openVPN CA cert: %v", err)
		}
	}

	var errs []error

	if err := r.ensureAPIServices(ctx, data); err != nil {
		errs = append(errs, err)
	}

	// We need to reconcile namespaces and services next to make sure
	// the openshift apiservices become available ASAP
	if err := r.reconcileNamespaces(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileServiceAccounts(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcilePodDisruptionBudgets(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileDeployments(ctx, data); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileServices(ctx, data); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileClusterRoles(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileClusterRoleBindings(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileRoles(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileRoleBindings(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileCRDs(ctx); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileConfigMaps(ctx, data); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileSecrets(ctx, data); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileDaemonSet(ctx, data); err != nil {
		errs = append(errs, err)
	}

	if err := r.reconcileValidatingWebhookConfigurations(ctx, data); err != nil {
		return err
	}

	if err := r.reconcileMutatingWebhookConfigurations(ctx, data); err != nil {
		errs = append(errs, err)
	}

	// This code supports switching between OpenVPN and Konnectivity setup (in both directions).
	// It can be removed one release after deprecating OpenVPN.
	if r.isKonnectivityEnabled {
		if err := r.ensureOpenVPNSetupIsRemoved(ctx); err != nil {
			errs = append(errs, err)
		}
	} else {
		if err := r.ensureKonnectivitySetupIsRemoved(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

func (r *reconciler) ensureAPIServices(ctx context.Context, data reconcileData) error {
	caCert := triple.EncodeCertPEM(data.caCert.Cert)
	creators := []reconciling.NamedAPIServiceCreatorGetter{
		metricsserver.APIServiceCreator(caCert),
	}

	if err := reconciling.ReconcileAPIServices(ctx, creators, metav1.NamespaceNone, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile APIServices: %v", err)
	}

	return nil
}

func (r *reconciler) reconcileServiceAccounts(ctx context.Context) error {
	creators := []reconciling.NamedServiceAccountCreatorGetter{
		userauth.ServiceAccountCreator(),
		usersshkeys.ServiceAccountCreator(),
		coredns.ServiceAccountCreator(),
		dnsautoscaler.ServiceAccountCreator(),
	}

	if r.nodeLocalDNSCache {
		creators = append(creators, nodelocaldns.ServiceAccountCreator())
	}

	if r.userSSHKeyAgent {
		creators = append(creators, usersshkeys.ServiceAccountCreator())
	}

	if err := reconciling.ReconcileServiceAccounts(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile ServiceAccounts in the namespace %s: %v", metav1.NamespaceSystem, err)
	}

	// Kubernetes Dashboard and related resources
	// Sys11 disable
	//creators = []reconciling.NamedServiceAccountCreatorGetter{
	//	kubernetesdashboard.ServiceAccountCreator(),
	//}
	//if err := reconciling.ReconcileServiceAccounts(ctx, creators, kubernetesdashboard.Namespace, r.Client); err != nil {
	//	return fmt.Errorf("failed to reconcile ServiceAccounts in the namespace %s: %v", kubernetesdashboard.Namespace, err)
	//}

	cloudInitSACreator := []reconciling.NamedServiceAccountCreatorGetter{
		cloudinitsettings.ServiceAccountCreator(),
	}
	if err := reconciling.ReconcileServiceAccounts(ctx, cloudInitSACreator, resources.CloudInitSettingsNamespace, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile cloud-init-getter in the namespace %s: %v", resources.CloudInitSettingsNamespace, err)
	}

	if r.isKonnectivityEnabled {
		creators = []reconciling.NamedServiceAccountCreatorGetter{
			konnectivity.ServiceAccountCreator(),
			metricsserver.ServiceAccountCreator(), // required only if metrics-server is running in user cluster
		}
		if err := reconciling.ReconcileServiceAccounts(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
			return fmt.Errorf("failed to reconcile ServiceAccounts in the namespace %s: %v", metav1.NamespaceSystem, err)
		}
	}

	return nil
}

func (r *reconciler) reconcileRoles(ctx context.Context) error {
	var cluster kubermaticv1.Cluster
	if err := r.seedClient.Get(ctx, ctrlruntimeclient.ObjectKey{Name: r.clusterName}, &cluster); err != nil {
		return fmt.Errorf("failed to reconcile the Roles: failed to get cluster: %v", err)
	}

	// kube-system
	creators := []reconciling.NamedRoleCreatorGetter{
		machinecontroller.KubeSystemRoleCreator(),
		clusterautoscaler.KubeSystemRoleCreator(),
	}

	if r.userSSHKeyAgent {
		creators = append(creators, usersshkeys.RoleCreator())
	}

	if cluster.Spec.Cloud.Openstack != nil {
		creators = append(creators, csiopenstack.RoleCreator())
	}

	if err := reconciling.ReconcileRoles(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile Roles in the namespace %s: %v", metav1.NamespaceSystem, err)
	}

	// kube-public
	creators = []reconciling.NamedRoleCreatorGetter{
		machinecontroller.ClusterInfoReaderRoleCreator(),
		machinecontroller.KubePublicRoleCreator(),
	}

	if err := reconciling.ReconcileRoles(ctx, creators, metav1.NamespacePublic, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile Roles in the namespace %s: %v", metav1.NamespacePublic, err)
	}

	// default
	creators = []reconciling.NamedRoleCreatorGetter{
		machinecontroller.EndpointReaderRoleCreator(),
		clusterautoscaler.DefaultRoleCreator(),
	}

	if err := reconciling.ReconcileRoles(ctx, creators, metav1.NamespaceDefault, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile Roles in the namespace %s: %v", metav1.NamespaceDefault, err)
	}

	// Kubernetes Dashboard and related resources
	// Sys11 disable
	//creators = []reconciling.NamedRoleCreatorGetter{
	//	kubernetesdashboard.RoleCreator(),
	//}
	//if err := reconciling.ReconcileRoles(ctx, creators, kubernetesdashboard.Namespace, r.Client); err != nil {
	//	return fmt.Errorf("failed to reconcile Roles in the namespace %s: %v", kubernetesdashboard.Namespace, err)
	//}

	cloudInitRoleCreator := []reconciling.NamedRoleCreatorGetter{
		cloudinitsettings.RoleCreator(),
	}
	if err := reconciling.ReconcileRoles(ctx, cloudInitRoleCreator, resources.CloudInitSettingsNamespace, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile cloud-init-getter role in the namespace %s: %v", resources.CloudInitSettingsNamespace, err)
	}

	return nil
}

func (r *reconciler) reconcileRoleBindings(ctx context.Context) error {
	var cluster kubermaticv1.Cluster
	if err := r.seedClient.Get(ctx, ctrlruntimeclient.ObjectKey{Name: r.clusterName}, &cluster); err != nil {
		return fmt.Errorf("failed to reconcile the RoleBindings: failed to get cluster: %v", err)
	}

	// kube-system
	creators := []reconciling.NamedRoleBindingCreatorGetter{
		machinecontroller.KubeSystemRoleBindingCreator(),
		metricsserver.RolebindingAuthReaderCreator(r.isKonnectivityEnabled),
		scheduler.RoleBindingAuthDelegator(),
		controllermanager.RoleBindingAuthDelegator(),
		clusterautoscaler.KubeSystemRoleBindingCreator(),
	}

	if r.userSSHKeyAgent {
		creators = append(creators, usersshkeys.RoleBindingCreator())
	}

	if cluster.Spec.Cloud.Openstack != nil {
		creators = append(creators, csiopenstack.RoleBindingCreator())
	}

	if err := reconciling.ReconcileRoleBindings(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile RoleBindings in kube-system Namespace: %v", err)
	}

	// kube-public
	creators = []reconciling.NamedRoleBindingCreatorGetter{
		machinecontroller.KubePublicRoleBindingCreator(),
		machinecontroller.ClusterInfoAnonymousRoleBindingCreator(),
	}
	if err := reconciling.ReconcileRoleBindings(ctx, creators, metav1.NamespacePublic, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile RoleBindings in kube-public Namespace: %v", err)
	}

	// Default
	creators = []reconciling.NamedRoleBindingCreatorGetter{
		machinecontroller.DefaultRoleBindingCreator(),
		clusterautoscaler.DefaultRoleBindingCreator(),
	}
	if err := reconciling.ReconcileRoleBindings(ctx, creators, metav1.NamespaceDefault, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile RoleBindings in default Namespace: %v", err)
	}

	// Kubernetes Dashboard and related resources
	// Sys11 disable
	//creators = []reconciling.NamedRoleBindingCreatorGetter{
	//	kubernetesdashboard.RoleBindingCreator(),
	//}
	//if err := reconciling.ReconcileRoleBindings(ctx, creators, kubernetesdashboard.Namespace, r.Client); err != nil {
	//	return fmt.Errorf("failed to reconcile RoleBindings in the namespace: %s: %v", kubernetesdashboard.Namespace, err)
	//}

	cloudInitRoleBindingCreator := []reconciling.NamedRoleBindingCreatorGetter{
		cloudinitsettings.RoleBindingCreator(),
	}
	if err := reconciling.ReconcileRoleBindings(ctx, cloudInitRoleBindingCreator, resources.CloudInitSettingsNamespace, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile cloud-init-getter RoleBindings in the namespace: %s: %v", resources.CloudInitSettingsNamespace, err)
	}

	return nil
}

func (r *reconciler) reconcileClusterRoles(ctx context.Context) error {
	var cluster kubermaticv1.Cluster
	if err := r.seedClient.Get(ctx, ctrlruntimeclient.ObjectKey{Name: r.clusterName}, &cluster); err != nil {
		return fmt.Errorf("failed to reconcile the ClusterRoles: failed to get cluster: %v", err)
	}

	creators := []reconciling.NamedClusterRoleCreatorGetter{
		kubestatemetrics.ClusterRoleCreator(),
		prometheus.ClusterRoleCreator(),
		machinecontroller.ClusterRoleCreator(),
		dnatcontroller.ClusterRoleCreator(),
		metricsserver.ClusterRoleCreator(),
		clusterautoscaler.ClusterRoleCreator(),
		// Sys11 disable kubernetesdashboard.ClusterRoleCreator(),
		coredns.ClusterRoleCreator(),
		dnsautoscaler.ClusterRolereator(),
	}
	if cluster.Spec.Cloud.Openstack != nil {
		creators = append(creators, csiopenstack.ClusterRoleCreator())
	}

	if err := reconciling.ReconcileClusterRoles(ctx, creators, "", r.Client); err != nil {
		return fmt.Errorf("failed to reconcile ClusterRoles: %v", err)
	}
	return nil
}

func (r *reconciler) reconcileClusterRoleBindings(ctx context.Context) error {
	var cluster kubermaticv1.Cluster
	if err := r.seedClient.Get(ctx, ctrlruntimeclient.ObjectKey{Name: r.clusterName}, &cluster); err != nil {
		return fmt.Errorf("failed to reconcile the ClusterRoleBindings: failed to get cluster: %v", err)
	}

	creators := []reconciling.NamedClusterRoleBindingCreatorGetter{
		userauth.ClusterRoleBindingCreator(),
		kubestatemetrics.ClusterRoleBindingCreator(),
		prometheus.ClusterRoleBindingCreator(),
		machinecontroller.ClusterRoleBindingCreator(),
		machinecontroller.NodeBootstrapperClusterRoleBindingCreator(),
		machinecontroller.NodeSignerClusterRoleBindingCreator(),
		dnatcontroller.ClusterRoleBindingCreator(),
		metricsserver.ClusterRoleBindingResourceReaderCreator(r.isKonnectivityEnabled),
		metricsserver.ClusterRoleBindingAuthDelegatorCreator(r.isKonnectivityEnabled),
		scheduler.ClusterRoleBindingAuthDelegatorCreator(),
		controllermanager.ClusterRoleBindingAuthDelegator(),
		clusterautoscaler.ClusterRoleBindingCreator(),
		systembasicuser.ClusterRoleBinding,
		cloudcontroller.ClusterRoleBindingCreator(),
		// Sys11 disable kubernetesdashboard.ClusterRoleBindingCreator(),
		coredns.ClusterRoleBindingCreator(),
		dnsautoscaler.ClusterRoleBindingCreator(),
	}
	if r.isKonnectivityEnabled {
		creators = append(creators, konnectivity.ClusterRoleBindingCreator())
	}

	if cluster.Spec.Cloud.Openstack != nil {
		creators = append(creators, csiopenstack.ClusterRoleBindingCreator())
	}

	if err := reconciling.ReconcileClusterRoleBindings(ctx, creators, "", r.Client); err != nil {
		return fmt.Errorf("failed to reconcile ClusterRoleBindings: %v", err)
	}
	return nil
}

func (r *reconciler) reconcileCRDs(ctx context.Context) error {
	creators := []reconciling.NamedCustomResourceDefinitionCreatorGetter{
		machinecontroller.MachineCRDCreator(),
		machinecontroller.MachineSetCRDCreator(),
		machinecontroller.MachineDeploymentCRDCreator(),
		machinecontroller.ClusterCRDCreator(),
	}

	if err := reconciling.ReconcileCustomResourceDefinitions(ctx, creators, "", r.Client); err != nil {
		return fmt.Errorf("failed to reconcile CustomResourceDefinitions: %v", err)
	}
	return nil
}

func (r *reconciler) reconcileMutatingWebhookConfigurations(ctx context.Context, data reconcileData) error {
	creators := []reconciling.NamedMutatingWebhookConfigurationCreatorGetter{
		machinecontroller.MutatingwebhookConfigurationCreator(data.caCert.Cert, r.namespace),
	}
	if err := reconciling.ReconcileMutatingWebhookConfigurations(ctx, creators, "", r.Client); err != nil {
		return fmt.Errorf("failed to reconcile MutatingWebhookConfigurations: %v", err)
	}
	return nil
}

func (r *reconciler) reconcileValidatingWebhookConfigurations(ctx context.Context, data reconcileData) error {
	creators := []reconciling.NamedValidatingWebhookConfigurationCreatorGetter{}

	if data.ccmMigration && data.csiCloudConfig != nil {
		creators = append(creators, csimigration.ValidatingwebhookConfigurationCreator(data.caCert.Cert, metav1.NamespaceSystem, resources.VsphereCSIMigrationWebhookConfigurationWebhookName))
	}

	if err := reconciling.ReconcileValidatingWebhookConfigurations(ctx, creators, "", r.Client); err != nil {
		return fmt.Errorf("failed to reconcile ValidatingWebhookConfigurations: %v", err)
	}
	return nil
}

func (r *reconciler) reconcileServices(ctx context.Context, data reconcileData) error {
	creatorsKubeSystem := []reconciling.NamedServiceCreatorGetter{
		coredns.ServiceCreator(r.dnsClusterIP),
	}

	if r.isKonnectivityEnabled {
		// metrics-server running in user cluster - ClusterIP service
		creatorsKubeSystem = append(creatorsKubeSystem, metricsserver.ServiceCreator(data.ipFamily))
	} else {
		// metrics-server running in seed cluster - ExternalName service
		creatorsKubeSystem = append(creatorsKubeSystem, metricsserver.ExternalNameServiceCreator(r.namespace))
	}

	if err := reconciling.ReconcileServices(ctx, creatorsKubeSystem, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile Services in kube-system namespace: %v", err)
	}

	// Kubernetes Dashboard and related resources
	// Sys11 disable
	//creators := []reconciling.NamedServiceCreatorGetter{
	//	kubernetesdashboard.ServiceCreator(),
	//}
	//if err := reconciling.ReconcileServices(ctx, creators, kubernetesdashboard.Namespace, r.Client); err != nil {
	//	return fmt.Errorf("failed to reconcile Services in namespace %s: %v", kubernetesdashboard.Namespace, err)
	//}

	return nil
}

func (r *reconciler) reconcileConfigMaps(ctx context.Context, data reconcileData) error {
	creators := []reconciling.NamedConfigMapCreatorGetter{
		machinecontroller.ClusterInfoConfigMapCreator(r.clusterURL.String(), data.caCert.Cert),
	}

	if err := reconciling.ReconcileConfigMaps(ctx, creators, metav1.NamespacePublic, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile ConfigMaps in kube-public namespace: %v", err)
	}

	if len(r.tunnelingAgentIP) > 0 {
		envoyConfig := envoyagent.Config{
			AdminPort: 9902,
			ProxyHost: r.clusterURL.Hostname(),
			ProxyPort: 8088,
			Listeners: []envoyagent.Listener{
				{
					BindAddress: r.tunnelingAgentIP.String(),
					BindPort:    r.kasSecurePort,
					Authority:   net.JoinHostPort(fmt.Sprintf("apiserver-external.%s.svc.cluster.local", r.namespace), "443"),
				},
			},
		}
		if !r.isKonnectivityEnabled {
			// add OpenVPN server port listener if Konnectivity is NOT enabled
			envoyConfig.Listeners = append(envoyConfig.Listeners,
				envoyagent.Listener{
					BindAddress: r.tunnelingAgentIP.String(),
					BindPort:    r.openvpnServerPort,
					Authority:   net.JoinHostPort(fmt.Sprintf("openvpn-server.%s.svc.cluster.local", r.namespace), "1194"),
				})
		}
		creators = []reconciling.NamedConfigMapCreatorGetter{
			cabundle.ConfigMapCreator(r.caBundle),
			envoyagent.ConfigMapCreator(envoyConfig),
		}
		if !r.isKonnectivityEnabled {
			creators = append(creators, openvpn.ClientConfigConfigMapCreator(r.tunnelingAgentIP.String(), r.openvpnServerPort))
		}
	} else {
		creators = []reconciling.NamedConfigMapCreatorGetter{
			cabundle.ConfigMapCreator(r.caBundle),
		}
		if !r.isKonnectivityEnabled {
			creators = append(creators, openvpn.ClientConfigConfigMapCreator(r.clusterURL.Hostname(), r.openvpnServerPort))
		}
	}

	creators = append(creators,
		coredns.ConfigMapCreator(r.clusterDNSDomain),
		nodelocaldns.ConfigMapCreator(r.dnsClusterIP))

	if r.nodeLocalDNSCache {
		creators = append(creators, nodelocaldns.ConfigMapCreator(r.dnsClusterIP))
	}

	if err := reconciling.ReconcileConfigMaps(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile ConfigMaps in kube-system namespace: %v", err)
	}

	return nil
}

func (r *reconciler) reconcileSecrets(ctx context.Context, data reconcileData) error {
	creators := []reconciling.NamedSecretCreatorGetter{
		cloudcontroller.CloudConfig(data.cloudConfig, resources.CloudConfigSecretName),
	}
	if !r.isKonnectivityEnabled {
		creators = append(creators, openvpn.ClientCertificate(data.openVPNCACert))
	} else {
		// required only if metrics-server is running in user cluster
		creators = append(creators, metricsserver.TLSServingCertSecretCreator(
			func() (*triple.KeyPair, error) {
				return data.caCert, nil
			}),
		)
	}

	if data.csiCloudConfig != nil {
		creators = append(creators, cloudcontroller.CloudConfig(data.csiCloudConfig, resources.CSICloudConfigSecretName))
		if data.ccmMigration {
			creators = append(creators, csimigration.TLSServingCertificateCreator(data.caCert))
		}
	}

	if r.userSSHKeyAgent {
		creators = append(creators, usersshkeys.SecretCreator(data.userSSHKeys))
	}

	if err := reconciling.ReconcileSecrets(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile Secrets in kube-system Namespace: %v", err)
	}

	// Kubernetes Dashboard and related resources
	// Sys11 disable
	//creators = []reconciling.NamedSecretCreatorGetter{
	//	kubernetesdashboard.KeyHolderSecretCreator(),
	//	kubernetesdashboard.CsrfTokenSecretCreator(),
	//}
	//
	//if err := reconciling.ReconcileSecrets(ctx, creators, kubernetesdashboard.Namespace, r.Client); err != nil {
	//	return fmt.Errorf("failed to reconcile Secrets in namespace %s: %v", kubernetesdashboard.Namespace, err)
	//}

	return nil
}

func (r *reconciler) reconcileDaemonSet(ctx context.Context, data reconcileData) error {
	var dsCreators []reconciling.NamedDaemonSetCreatorGetter

	var cluster kubermaticv1.Cluster
	if err := r.seedClient.Get(ctx, ctrlruntimeclient.ObjectKey{Name: r.clusterName}, &cluster); err != nil {
		return fmt.Errorf("failed to reconcile the DaemonSet: failed to get cluster: %v", err)
	}

	// may be nil
	nodeLocalDNSResourcesOverride := cluster.Spec.ComponentsOverride.NodeLocalDNS.Resources
	if r.nodeLocalDNSCache {
		dsCreators = append(dsCreators, nodelocaldns.DaemonSetCreator(nodeLocalDNSResourcesOverride))
	}

	if r.userSSHKeyAgent {
		dsCreators = append(dsCreators, usersshkeys.DaemonSetCreator(r.versions))
	}

	if len(r.tunnelingAgentIP) > 0 {
		configHash, err := r.getEnvoyAgentConfigHash(ctx)
		if err != nil {
			return fmt.Errorf("failed to retrieve envoy-agent config hash: %w", err)
		}
		dsCreators = append(dsCreators, envoyagent.DaemonSetCreator(r.tunnelingAgentIP, r.versions, configHash))
	}

	if err := reconciling.ReconcileDaemonSets(ctx, dsCreators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile the DaemonSet: %v", err)
	}

	return nil
}

func (r *reconciler) reconcileNamespaces(ctx context.Context) error {

	creators := []reconciling.NamedNamespaceCreatorGetter{
		// Sys11 disable kubernetesdashboard.NamespaceCreator,
		cloudinitsettings.NamespaceCreator,
	}
	if err := reconciling.ReconcileNamespaces(ctx, creators, "", r.Client); err != nil {
		return fmt.Errorf("failed to reconcile namespaces: %v", err)
	}
	return nil
}

func (r *reconciler) reconcileDeployments(ctx context.Context, data reconcileData) error {
	// Kubernetes Dashboard and related resources
	// Sys11 disable
	//creators := []reconciling.NamedDeploymentCreatorGetter{
	//	kubernetesdashboard.DeploymentCreator(),
	//}
	//if err := reconciling.ReconcileDeployments(ctx, creators, kubernetesdashboard.Namespace, r.Client); err != nil {
	//	return fmt.Errorf("failed to reconcile Deployments in namespace %s: %v", kubernetesdashboard.Namespace, err)
	//}

	dnsAutescalerCreators := []reconciling.NamedDeploymentCreatorGetter{dnsautoscaler.DeploymentCreator()}
	if err := reconciling.ReconcileDeployments(ctx, dnsAutescalerCreators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile dns autoscaler Deployment in namespace %s: %v", metav1.NamespaceSystem, err)
	}

	// DNS autoscaler changes number of replicas of the coredns deployment,
	// to make sure we won't recreate coredns when replicas number changed we query
	// and set current replicas number. We can do that through modifiers.
	doNotReconcilesOnReplicaChangeModifier := reconciling.SetRunningReplicasNumberIfExistsWrapper(ctx, r.Client)
	corednsCreators := []reconciling.NamedDeploymentCreatorGetter{coredns.DeploymentCreator(r.clusterSemVer)}
	if err := reconciling.ReconcileDeployments(ctx, corednsCreators, metav1.NamespaceSystem, r.Client, doNotReconcilesOnReplicaChangeModifier); err != nil {
		return fmt.Errorf("failed to reconcile coredns Deployment in namespace %s: %v", metav1.NamespaceSystem, err)
	}

	if r.isKonnectivityEnabled {
		creators := []reconciling.NamedDeploymentCreatorGetter{
			konnectivity.DeploymentCreator(r.konnectivityServerHost, r.konnectivityServerPort),
			metricsserver.DeploymentCreator(), // deploy metrics-server in user cluster
		}
		if err := reconciling.ReconcileDeployments(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
			return fmt.Errorf("failed to reconcile Deployments in namespace %s: %w", metav1.NamespaceSystem, err)
		}
	}

	return nil
}

func (r *reconciler) reconcilePodDisruptionBudgets(ctx context.Context) error {
	creators := []reconciling.NamedPodDisruptionBudgetCreatorGetter{
		coredns.PodDisruptionBudgetCreator(),
	}
	if r.isKonnectivityEnabled {
		creators = append(creators,
			konnectivity.PodDisruptionBudgetCreator(),
			metricsserver.PodDisruptionBudgetCreator(),
		)
	}
	if err := reconciling.ReconcilePodDisruptionBudgets(ctx, creators, metav1.NamespaceSystem, r.Client); err != nil {
		return fmt.Errorf("failed to reconcile PodDisruptionBudgets: %w", err)
	}
	return nil
}

type reconcileData struct {
	caCert        *triple.KeyPair
	openVPNCACert *resources.ECDSAKeyPair
	userSSHKeys   map[string][]byte
	cloudConfig   []byte
	// csiCloudConfig is currently used only by vSphere, whose needs it to properly configure the external CSI driver
	csiCloudConfig  []byte
	ccmMigration    bool
	ipFamily        kubermaticv1.IPFamily
	clusterAddress  *kubermaticv1.ClusterAddress
	k8sServiceAPIIP *net.IP
}

func (r *reconciler) ensureOpenVPNSetupIsRemoved(ctx context.Context) error {
	for _, resource := range openvpn.ResourcesForDeletion() {
		err := r.Client.Delete(ctx, resource)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to ensure OpenVPN resources are removed/not present: %w", err)
		}
	}
	return nil
}

func (r *reconciler) getEnvoyAgentConfigHash(ctx context.Context) (string, error) {
	cm := corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: resources.EnvoyAgentConfigMapName, Namespace: metav1.NamespaceSystem}, &cm)
	if err != nil {
		return "", fmt.Errorf("failed to get envoy-agent configmap: %w", err)
	}
	configHash := sha1.New()
	configHash.Write([]byte(cm.Data[resources.EnvoyAgentConfigFileName]))
	return fmt.Sprintf("%x", configHash.Sum(nil)), nil
}

func (r *reconciler) ensureKonnectivitySetupIsRemoved(ctx context.Context) error {
	for _, resource := range konnectivity.ResourcesForDeletion() {
		err := r.Client.Delete(ctx, resource)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to ensure Konnectivity resources are removed/not present: %w", err)
		}
	}
	for _, resource := range metricsserver.UserClusterResourcesForDeletion() {
		err := r.Client.Delete(ctx, resource)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to ensure metrics-server resources are removed/not present: %w", err)
		}
	}
	return nil
}
