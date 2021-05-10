package common

import (
	"context"
	"fmt"
	"strings"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"
	"k8c.io/kubermatic/v2/pkg/keycloak"
	"k8c.io/kubermatic/v2/pkg/provider"
	"k8c.io/kubermatic/v2/pkg/resources"
	kcerrors "k8c.io/kubermatic/v2/pkg/util/errors"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

//nolint:goconst
func GetSysElevenOidcKubeconfigEndpoint(ctx context.Context, projectID, clusterID string, useName bool, projectProvider provider.ProjectProvider, clusterProvider provider.ClusterProvider, userInfoGetter provider.UserInfoGetter) (interface{}, error) {
	userInfo, err := userInfoGetter(ctx, projectID)
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}
	project, err := projectProvider.Get(userInfo, projectID, &provider.ProjectGetOptions{})
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}

	cluster, err := clusterProvider.Get(userInfo, clusterID, &provider.ClusterGetOptions{})
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}
	oidcClientCfg, err := clusterProvider.GetAdminKubeconfigForCustomerCluster(cluster)
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}

	oidcSettings, err := getOidcSettings(ctx, cluster)
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}

	clientCmdAuth := clientcmdapi.NewAuthInfo()
	clientCmdAuthProvider := &clientcmdapi.AuthProviderConfig{Config: map[string]string{}}
	clientCmdAuthProvider.Name = "oidc"
	clientCmdAuthProvider.Config["idp-issuer-url"] = oidcSettings.IssuerURL
	clientCmdAuthProvider.Config["client-id"] = oidcSettings.ClientID
	if oidcSettings.ClientSecret != "" {
		clientCmdAuthProvider.Config["client-secret"] = oidcSettings.ClientSecret
	}
	if oidcSettings.ExtraScopes != "" {
		clientCmdAuthProvider.Config["extra-scopes"] = oidcSettings.ExtraScopes
	}
	clientCmdAuth.AuthProvider = clientCmdAuthProvider

	oidcClientCfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	oidcClientCfg.AuthInfos[resources.KubeconfigDefaultContextKey] = clientCmdAuth

	if useName {
		return getKubeconfigWithUniqueName(project, cluster, oidcClientCfg, "oidc")
	}
	return &encodeKubeConfigResponse{clientCfg: oidcClientCfg, filePrefix: "oidc"}, nil
}

func GetKubeLoginKubeconfigEndpoint(ctx context.Context, projectID, clusterID string, useName bool, projectProvider provider.ProjectProvider, clusterProvider provider.ClusterProvider, userInfoGetter provider.UserInfoGetter) (interface{}, error) {
	userInfo, err := userInfoGetter(ctx, projectID)
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}
	project, err := projectProvider.Get(userInfo, projectID, &provider.ProjectGetOptions{})
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}

	cluster, err := clusterProvider.Get(userInfo, clusterID, &provider.ClusterGetOptions{})
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}
	kubeloginClientCfg, err := clusterProvider.GetAdminKubeconfigForCustomerCluster(cluster)
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}

	oidcSettings, err := getOidcSettings(ctx, cluster)
	if err != nil {
		return nil, common.KubernetesErrorToHTTPError(err)
	}

	clientCmdAuth := clientcmdapi.NewAuthInfo()
	execConfig := &clientcmdapi.ExecConfig{
		APIVersion: "client.authentication.k8s.io/v1beta1",
		Command:    "kubectl",
		Args: []string{
			"oidc-login",
			"get-token",
			fmt.Sprintf("--oidc-issuer-url=%s", oidcSettings.IssuerURL),
			fmt.Sprintf("--oidc-client-id=%s", oidcSettings.ClientID),
		},
	}
	if oidcSettings.ClientSecret != "" {
		execConfig.Args = append(execConfig.Args, fmt.Sprintf("--oidc-client-secret=%s", oidcSettings.ClientSecret))
	}
	if oidcSettings.ExtraScopes != "" {
		for _, scope := range strings.Split(oidcSettings.ExtraScopes, " ") {
			execConfig.Args = append(execConfig.Args, fmt.Sprintf("--oidc-extra-scope=%s", scope))
		}
	}
	clientCmdAuth.Exec = execConfig

	kubeloginClientCfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	kubeloginClientCfg.AuthInfos[resources.KubeconfigDefaultContextKey] = clientCmdAuth

	if useName {
		return getKubeconfigWithUniqueName(project, cluster, kubeloginClientCfg, "kubelogin")
	}
	return &encodeKubeConfigResponse{clientCfg: kubeloginClientCfg, filePrefix: "kubelogin"}, nil
}

func getKubeconfigWithUniqueName(project *kubermaticv1.Project, cluster *kubermaticv1.Cluster, clientConfig *clientcmdapi.Config, filePrefix string) (interface{}, error) {
	kubeloginClientCfg, err := NoDefaultsKubeconfig(clientConfig, "oidc", cluster, project)
	if err != nil {
		return nil, kcerrors.NewBadRequest("failed to replace default names in kubelogin kubeconfig: %v", err)
	}
	return &encodeKubeConfigResponse{
		clientCfg:   kubeloginClientCfg,
		filePrefix:  filePrefix,
		clusterName: fmt.Sprintf("%s-%s", project.Spec.Name, cluster.Spec.HumanReadableName),
	}, nil
}

func getOidcSettings(ctx context.Context, cluster *kubermaticv1.Cluster) (*kubermaticv1.OIDCSettings, error) {
	keycloakFacade := ctx.Value(middleware.KeycloakFacadeContextKey).(keycloak.Facade)

	switch {
	case cluster.Spec.Sys11Auth.Realm != "":
		settings, err := keycloak.Sys11AuthToOidcSettings(cluster.Spec.Sys11Auth, keycloakFacade)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		return settings, nil
	case cluster.Spec.OIDC.IssuerURL != "" && cluster.Spec.OIDC.ClientID != "":
		return &cluster.Spec.OIDC, nil
	default:
		return nil, kcerrors.NewNotFound("OIDC settings for", cluster.Name)
	}
}

// This function will replace the 'default' context and user in a kubeconfig with sanitized names. Error will for now
// always be nil, but added the return in case we want to add checks e.g. for cluster or project in the future
func NoDefaultsKubeconfig(clientConfig *clientcmdapi.Config, username string, cluster *kubermaticv1.Cluster, project *kubermaticv1.Project) (*clientcmdapi.Config, error) {

	newUsername := fmt.Sprintf("%s-%s", username, cluster.Name)
	newContext := fmt.Sprintf("%s@%s/%s", username, project.Spec.Name, cluster.Spec.HumanReadableName)

	clientConfig.AuthInfos[newUsername] = clientConfig.AuthInfos[resources.KubeconfigDefaultContextKey]
	delete(clientConfig.AuthInfos, resources.KubeconfigDefaultContextKey)

	clientConfig.Contexts[newContext] = clientConfig.Contexts[resources.KubeconfigDefaultContextKey]
	clientConfig.Contexts[newContext].AuthInfo = newUsername
	delete(clientConfig.Contexts, resources.KubeconfigDefaultContextKey)

	clientConfig.CurrentContext = newContext

	return clientConfig, nil
}

func getOpenstackBillingTenant(cloudSpec *kubermaticv1.CloudSpec) string {
	if cloudSpec.AWS != nil {
		return cloudSpec.AWS.OpenstackBillingTenant
	}
	if cloudSpec.Azure != nil {
		return cloudSpec.Azure.OpenstackBillingTenant
	}
	if cloudSpec.Openstack != nil {
		return cloudSpec.Openstack.Tenant
	}
	return ""
}
