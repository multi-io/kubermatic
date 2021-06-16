package handler

import (
	"context"
	goerrors "errors"
	"fmt"

	"k8s.io/client-go/rest"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"
	"k8c.io/kubermatic/v2/pkg/provider"
	k8cerrors "k8c.io/kubermatic/v2/pkg/util/errors"
)

// RawResponse is the default representation of a raw (proxied) HTTP response
type RawResponse struct {
	Header http.Header
	Body   []byte
}

func (r Routing) prometheusProxyHandler() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetPrivilegedClusterProvider(r.clusterProviderGetter, r.seedsGetter),
		)(getPrometheusProxyEndpoint(r.seedsGetter, r.userInfoGetter)),
		decodePrometheusProxyReq,
		encodeRawResponse,
		r.defaultServerOptions()...,
	)
}

// GetPrometheusProxyReq represents a request to the Prometheus proxy route
type GetPrometheusProxyReq struct {
	common.GetClusterReq
	PrometheusQueryPath string              `json:"prometheus_query_path"`
	PrometheusQuery     map[string][]string `json:"prometheus_raw_query"`
	RequestHeaders      http.Header
}

func decodePrometheusProxyReq(c context.Context, r *http.Request) (interface{}, error) {
	var req GetPrometheusProxyReq

	clusterID, err := common.DecodeClusterID(c, r)
	if err != nil {
		return nil, err
	}

	dcr, err := common.DecodeDcReq(c, r)
	if err != nil {
		return nil, err
	}

	req.ClusterID = clusterID
	req.DCReq = dcr.(common.DCReq)
	req.PrometheusQueryPath = mux.Vars(r)["query_path"]
	if req.PrometheusQueryPath != "query" && req.PrometheusQueryPath != "query_range" {
		return nil, k8cerrors.New(http.StatusBadRequest, "invalid Prometheus query path")
	}

	req.PrometheusQuery = r.URL.Query()
	req.RequestHeaders = r.Header

	return req, nil
}

func getPrometheusProxyEndpoint(seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		privilegedClusterProvider := ctx.Value(middleware.PrivilegedClusterProviderContextKey).(provider.PrivilegedClusterProvider)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		req := request.(GetPrometheusProxyReq)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		c, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		prometheusQuery := map[string]string{}

		// prometheus does not support the same query parameter twice, so we can just use the first
		for k, v := range req.PrometheusQuery {
			prometheusQuery[k] = v[0]
		}

		seedAdminClient := privilegedClusterProvider.GetSeedClusterAdminClient()
		proxyRequest := seedAdminClient.CoreV1().Services(fmt.Sprintf("cluster-%s", c.Name)).ProxyGet(
			"http",
			"prometheus",
			"web",
			"/api/v1/"+req.PrometheusQueryPath,
			prometheusQuery,
		).(*rest.Request)

		body, err := proxyRequest.DoRaw(ctx)

		if err != nil {
			return nil, err
		}

		resp := RawResponse{
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: body,
		}

		return resp, nil
	}
}

func encodeRawResponse(c context.Context, w http.ResponseWriter, response interface{}) error {
	if resp, ok := response.(RawResponse); ok {
		for field, values := range resp.Header {
			for _, value := range values {
				w.Header().Set(field, value)
			}
		}
		_, err := w.Write(resp.Body)
		return err
	}
	return goerrors.New("internal error (unexpected raw response object)")
}
