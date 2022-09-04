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

package v2

import (
	"net/http"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"

	"k8c.io/kubermatic/v2/pkg/handler"
	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"
	"k8c.io/kubermatic/v2/pkg/handler/v2/provider"
)

// RegisterV2SysEleven declares all Sys11 router paths for v2
func (r Routing) RegisterV2SysEleven(mux *mux.Router, metrics common.ServerMetrics) {

	mux.Methods(http.MethodGet).
		Path("/projects/{project_id}/clusters/{cluster_id}/providers/openstack/images").
		Handler(r.listOpenstackImagesNoCredentials())
}

// swagger:route GET /api/v2/projects/{project_id}/clusters/{cluster_id}/providers/openstack/images openstack listOpenstackImagesNoCredentialsV2
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
		)(provider.OpenstackImageWithClusterCredentialsEndpoint(r.projectProvider, r.privilegedProjectProvider, r.seedsGetter,
			r.userInfoGetter, r.caBundle)),
		provider.DecodeOpenstackNoCredentialsReq,
		handler.EncodeJSON,
		r.defaultServerOptions()...,
	)
}
