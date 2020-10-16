package handler

import (
	"net/http"

	"github.com/kubermatic/kubermatic/api/pkg/handler/v1/addon"
	"github.com/kubermatic/kubermatic/api/pkg/handler/v1/node"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/kubermatic/kubermatic/api/pkg/handler/middleware"
	"github.com/kubermatic/kubermatic/api/pkg/handler/v1/cluster"
	"github.com/kubermatic/kubermatic/api/pkg/handler/v1/provider"

	"github.com/gorilla/mux"
)

// RegisterV1SysEleven registers additional endpoints used by SysEleven.
func (r Routing) RegisterV1SysEleven(mux *mux.Router) {
	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/cluster/{cluster_id}/prometheus/{query_path}").
		Handler(r.prometheusProxyHandler())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/usage").
		Handler(r.resourceUsageCollectorProxyHandler())

	mux.Methods(http.MethodGet).
		Path("/providers/openstack/images").
		Handler(r.listOpenstackImages())

	mux.Methods(http.MethodGet).
		Path("/providers/openstack/quotalimits").
		Handler(r.listOpenstackQuotaLimits())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/providers/openstack/images").
		Handler(r.listOpenstackImagesNoCredentials())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/providers/openstack/quotalimits").
		Handler(r.listOpenstackQuotaLimitsNoCredentials())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/oidckubeconfig").
		Handler(r.getOidcClusterKubeconfig())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/kubeloginkubeconfig").
		Handler(r.getKubeLoginClusterKubeconfig())

	mux.Methods(http.MethodPost).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons").
		Handler(r.createAddon())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons").
		Handler(r.listAddons())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons/{addon_id}").
		Handler(r.getAddon())

	mux.Methods(http.MethodPatch).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons/{addon_id}").
		Handler(r.patchAddon())

	mux.Methods(http.MethodDelete).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons/{addon_id}").
		Handler(r.deleteAddon())

	mux.Methods(http.MethodPost).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests").
		Handler(r.createNodeDeploymentRequest())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests").
		Handler(r.listNodeDeploymentRequests())

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests/{ndrequest_id}").
		Handler(r.getNodeDeploymentRequest())

	mux.Methods(http.MethodPatch).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests/{ndrequest_id}").
		Handler(r.patchNodeDeploymentRequest())

	mux.Methods(http.MethodDelete).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests/{ndrequest_id}").
		Handler(r.deleteNodeDeploymentRequest())

	// Metakube Vault Unsealing
	mux.Methods(http.MethodPost).
		Path("/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons/metakube-vault/unseal").
		Handler(r.unsealVaultAddon())
}

// swagger:route GET /api/v1/providers/openstack/images openstack listOpenstackImages
//
// Lists images from openstack
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: []Image
func (r Routing) listOpenstackImages() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
		)(provider.OpenstackImageEndpoint(r.seedsGetter, r.presetsProvider, r.userInfoGetter)),
		provider.DecodeOpenstackReq,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route GET /api/v1/providers/openstack/quotalimits openstack listOpenstackQuotaLimits
//
// Lists quotalimits for tenant from openstack
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: Quotas
func (r Routing) listOpenstackQuotaLimits() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
		)(provider.OpenstackQuotaLimitEndpoint(r.seedsGetter, r.presetsProvider, r.userInfoGetter)),
		provider.DecodeOpenstackReq,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route GET /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/providers/openstack/images openstack listOpenstackImagesNoCredentials
//
// Lists images from openstack
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: []Image
func (r Routing) listOpenstackImagesNoCredentials() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.SetPrivilegedClusterProvider(r.clusterProviderGetter, r.seedsGetter),
		)(provider.OpenstackImageNoCredentialsEndpoint(r.projectProvider, r.privilegedProjectProvider, r.seedsGetter, r.userInfoGetter)),
		provider.DecodeOpenstackNoCredentialsReq,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route GET /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/providers/openstack/quotalimits openstack listOpenstackQuotaLimitsNoCredentials
//
// Lists quotalimits for tenant from openstack
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: Quotas
func (r Routing) listOpenstackQuotaLimitsNoCredentials() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.SetPrivilegedClusterProvider(r.clusterProviderGetter, r.seedsGetter),
		)(provider.OpenstackQuotaLimitNoCredentialsEndpoint(r.projectProvider, r.privilegedProjectProvider, r.seedsGetter, r.userInfoGetter)),
		provider.DecodeOpenstackNoCredentialsReq,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route GET /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/kubeloginkubeconfig project getKubeLoginClusterKubeconfig
//
//     Gets the kubeconfig for the specified cluster with oidc authentication that works nicely with kube-login.
//
//     Produces:
//     - application/yaml
//
//     Responses:
//       default: errorResponse
//       200: Kubeconfig
//       401: empty
//       403: empty
func (r Routing) getKubeLoginClusterKubeconfig() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.Keycloak(r.keycloakFacade),
		)(cluster.GetKubeLoginKubeconfigEndpoint(r.projectProvider, r.userInfoGetter)),
		cluster.DecodeGetAdminKubeconfig,
		cluster.EncodeKubeconfig,
		r.defaultServerOptions()...,
	)
}

// swagger:route POST /projects/{project_id}/dc/{dc}/clusters/{cluster_id}/addons/metakube-vault/unseal
//
//    Unseals the vault addon in the cluster.
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: empty
//       401: empty
//       403: empty
func (r Routing) unsealVaultAddon() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.Addons(r.addonProviderGetter, r.seedsGetter),
		)(addon.UnsealVaultAddonEndpoint(r.seedsGetter, r.userInfoGetter)),
		addon.DecodeUnsealVaultAddon,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route POST /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests project createNodeDeploymentRequest
//
//     Creates a NodeDeploymentRequest that will belong to the given cluster
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       201: NodeDeploymentRequest
//       401: empty
//       403: empty
func (r Routing) createNodeDeploymentRequest() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.MachineDeploymentRequests(r.mdRequestProviderGetter, r.seedsGetter),
		)(node.CreateNodeDeploymentRequestEndpoint(r.sshKeyProvider, r.projectProvider, r.seedsGetter, r.userInfoGetter)),
		node.DecodeCreateNodeDeploymentRequest,
		setStatusCreatedHeader(encodeJSON),
		r.defaultServerOptions()...,
	)
}

// swagger:route GET /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests project listNodeDeploymentRequests
//
//     Lists NodeDeploymentRequests that belong to the given cluster
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: []NodeDeploymentRequest
//       401: empty
//       403: empty
func (r Routing) listNodeDeploymentRequests() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.MachineDeploymentRequests(r.mdRequestProviderGetter, r.seedsGetter),
		)(node.ListNodeDeploymentRequestEndpoint(r.projectProvider, r.userInfoGetter)),
		node.DecodeListNodeDeploymentRequests,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route GET /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests/{ndrequest_id} project getNodeDeploymentRequest
//
//     Gets a NodeDeploymentRequest that is assigned to the given cluster.
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: NodeDeploymentRequest
//       401: empty
//       403: empty
func (r Routing) getNodeDeploymentRequest() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.MachineDeploymentRequests(r.mdRequestProviderGetter, r.seedsGetter),
		)(node.GetNodeDeploymentRequestEndpoint(r.projectProvider, r.userInfoGetter)),
		node.DecodeGetNodeDeploymentRequest,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route PATCH /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests/{ndrequest_id} project patchNodeDeploymentRequest
//
//     Patches a NodeDeploymentRequest that is assigned to the given cluster.
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: NodeDeploymentRequest
//       401: empty
//       403: empty
func (r Routing) patchNodeDeploymentRequest() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.MachineDeploymentRequests(r.mdRequestProviderGetter, r.seedsGetter),
		)(node.PatchNodeDeploymentRequestEndpoint(r.sshKeyProvider, r.projectProvider, r.seedsGetter, r.userInfoGetter)),
		node.DecodePatchNodeDeploymentRequest,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}

// swagger:route DELETE /api/v1/projects/{project_id}/dc/{dc}/clusters/{cluster_id}/ndrequests/{ndrequest_id} project deleteNodeDeploymentRequest
//
//    Deletes the given NodeDeploymentRequest that belongs to the cluster.
//
//     Produces:
//     - application/json
//
//     Responses:
//       default: errorResponse
//       200: empty
//       401: empty
//       403: empty
func (r Routing) deleteNodeDeploymentRequest() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
			middleware.MachineDeploymentRequests(r.mdRequestProviderGetter, r.seedsGetter),
		)(node.DeleteNodeDeploymentRequestEndpoint(r.projectProvider, r.userInfoGetter)),
		node.DecodeGetNodeDeploymentRequest,
		encodeJSON,
		r.defaultServerOptions()...,
	)
}
