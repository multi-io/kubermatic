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

package cluster

import (
	"context"
	"net/http"

	"github.com/go-kit/kit/endpoint"

	handlercommon "k8c.io/kubermatic/v2/pkg/handler/common"
	"k8c.io/kubermatic/v2/pkg/provider"
)

func GetAdminKubeconfigEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetClusterKubeconfigRequest)
		return handlercommon.GetAdminKubeconfigEndpoint(ctx, userInfoGetter, req.ProjectID, req.ClusterID, req.UseUniqueNames, projectProvider, privilegedProjectProvider)
	}
}

func EncodeKubeconfig(c context.Context, w http.ResponseWriter, response interface{}) (err error) {
	return handlercommon.EncodeKubeconfig(c, w, response)
}

func GetOidcKubeconfigEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetClusterReq)
		return handlercommon.GetOidcKubeconfigEndpoint(ctx, userInfoGetter, req.ProjectID, req.ClusterID, projectProvider, privilegedProjectProvider)
	}
}

func EncodeOIDCKubeconfig(c context.Context, w http.ResponseWriter, response interface{}) (err error) {
	return handlercommon.EncodeOIDCKubeconfig(c, w, response)
}

func DecodeGetAdminKubeconfig(c context.Context, r *http.Request) (interface{}, error) {
	var req GetClusterKubeconfigRequest

	clusterReq, err := DecodeGetClusterReq(c, r)
	if err != nil {
		return nil, err
	}
	req.GetClusterReq = clusterReq.(GetClusterReq)
	req.UseUniqueNames = r.URL.Query().Get("use-unique-names") == "yes"

	return req, nil
}

func DecodeCreateOIDCKubeconfig(c context.Context, r *http.Request) (interface{}, error) {
	return handlercommon.DecodeCreateOIDCKubeconfig(c, r)
}
