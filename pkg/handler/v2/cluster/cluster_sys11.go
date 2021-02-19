package cluster

import (
	"context"

	handlercommon "k8c.io/kubermatic/v2/pkg/handler/common"
	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/provider"

	"github.com/go-kit/kit/endpoint"
)

//nolint:goconst
func GetSysElevenOidcKubeconfigEndpoint(projectProvider provider.ProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetClusterKubeconfigRequest)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		return handlercommon.GetSysElevenOidcKubeconfigEndpoint(ctx, req.ProjectID, req.ClusterID, req.UseUniqueNames, projectProvider, clusterProvider, userInfoGetter)
	}
}

func GetKubeLoginKubeconfigEndpoint(projectProvider provider.ProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(GetClusterKubeconfigRequest)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		return handlercommon.GetKubeLoginKubeconfigEndpoint(ctx, req.ProjectID, req.ClusterID, req.UseUniqueNames, projectProvider, clusterProvider, userInfoGetter)
	}
}

type GetClusterKubeconfigRequest struct {
	GetClusterReq
	// in: path
	UseUniqueNames bool `json:"use_unique_names"`
}
