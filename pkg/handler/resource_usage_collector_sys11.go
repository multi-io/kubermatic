package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"time"

	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"net/http"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"
	"k8c.io/kubermatic/v2/pkg/provider"
)

var resourceUsageCollectorAuthToken = os.Getenv("RESOURCE_USAGE_COLLECTOR_AUTH_TOKEN")
var csvColumns = []string{"time", "cluster_id", "project_id", "seed_cluster_name", "datacenter_name", "cluster_owner", "customer_project", "cpu", "memory", "floating_ips"}

// swagger:route GET /api/v1/projects/{project_id}/dc/{dc}/cluster/{cluster_id}/usage
//
// Gets resource usage data for the cluster
//
//     Produces:
//     - application/json
//     - txt/csv
//
//     Responses:
//       default: errorResponse
//       200: []UsageDataRecord
func (r Routing) resourceUsageCollectorProxyHandler() http.Handler {
	return httptransport.NewServer(
		endpoint.Chain(
			middleware.TokenVerifier(r.tokenVerifiers, r.userProvider),
			middleware.UserSaver(r.userProvider),
			middleware.SetClusterProvider(r.clusterProviderGetter, r.seedsGetter),
		)(getResourceUsageCollectorProxyEndpoint(r.seedsGetter, r.userInfoGetter)),
		decodeResourceUsageCollectorProxyReq,
		encodeRawResponse,
		r.defaultServerOptions()...,
	)
}

// GetResourceUsageCollectorProxyReq represents a request to the ResourceUsageCollector proxy route
type GetResourceUsageCollectorProxyReq struct {
	common.GetClusterReq
	from     string
	until    string
	fileType string
}

// NodeRecord represents metrics collected for each worker node of the cluster.
type NodeRecord struct {
	ID          string `json:"id"`
	CPU         int    `json:"cpu"`
	FloatingIps int    `json:"floating_ips"`
	Memory      int    `json:"memory"`
}

// Metrics respresents the metrics collected as part of
// each UsageDataRecord for a cluster.
type Metrics struct {
	Nodes         []NodeRecord     `json:"nodes"`
	Storage       int              `json:"storage"`
	Addons        []string         `json:"addons"`
	Loadbalancers []map[string]int `json:"loadbalancers"`
}

// UsageDataRecord represents a usage record from resource-usage-collector API.
type UsageDataRecord struct {
	ID              int     `json:"id"`
	Time            string  `json:"time"`
	ClusterID       string  `json:"cluster_id"`
	ProjectID       string  `json:"project_id"`
	SeedClusterName string  `json:"seed_cluster_name"`
	DatacenterName  string  `json:"datacenter_name"`
	ClusterOwner    string  `json:"cluster_owner"`
	CustomerProject string  `json:"customer_project"`
	Metrics         Metrics `json:"metrics"`
}

// Decodes HTTP request into type GetResourceUsageCollectorProxyReq
func decodeResourceUsageCollectorProxyReq(c context.Context, r *http.Request) (interface{}, error) {
	var req GetResourceUsageCollectorProxyReq

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
	req.from = r.URL.Query().Get("from")
	req.until = r.URL.Query().Get("until")
	req.fileType = r.URL.Query().Get(("fileType"))

	return req, nil
}

// gets usage data from resource-usage-collector service and generates RawResponse based on query parameters. This RawResponse is then
// returned
func getResourceUsageCollectorProxyEndpoint(seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		req := request.(GetResourceUsageCollectorProxyReq)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		c, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		seed, _, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, c.Spec.Cloud.DatacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		masterConfig, err := rest.InClusterConfig()
		if err != nil {
			// For running outside of a cluster
			masterConfig, err = clientcmd.
				NewNonInteractiveDeferredLoadingClientConfig(
					clientcmd.NewDefaultClientConfigLoadingRules(),
					&clientcmd.ConfigOverrides{},
				).ClientConfig()
			if err != nil {
				return nil, fmt.Errorf("failed to create in cluster client config: %v", err)
			}
		}

		masterKubeClient, err := kubernetes.NewForConfig(masterConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create a master kubernetes client: %v", err)
		}

		resourceUsageCollectorQuery := map[string]string{}

		// Fixed Query parameters can be generated from the given url
		resourceUsageCollectorQuery["seed_cluster_name"] = seed.Name
		resourceUsageCollectorQuery["cluster_id"] = req.ClusterID
		resourceUsageCollectorQuery["project_id"] = req.ProjectID
		if req.from != "" {
			resourceUsageCollectorQuery["from"] = req.from
		}
		if req.until != "" {
			resourceUsageCollectorQuery["until"] = req.until
		}

		proxyRequest := masterKubeClient.CoreV1().Services("resource-usage-collector").ProxyGet(
			"http",
			"resource-usage-collector",
			"http",
			"/v1/usage",
			resourceUsageCollectorQuery,
		).(*rest.Request)

		// set context timeout incase the external service is not responding
		ctx, cancel := context.WithTimeout(ctx, time.Second*100)
		defer cancel()

		proxyRequest.SetHeader("x-auth", resourceUsageCollectorAuthToken)
		body, err := proxyRequest.DoRaw(ctx)

		if err != nil {
			return nil, err
		}

		if req.fileType == "json" {
			filename := fmt.Sprintf("cluster-%s-%s-%s.json", req.ClusterID, req.from, req.until)
			resp := RawResponse{
				Header: http.Header{
					"Content-Type":        []string{"txt/json"},
					"Content-disposition": []string{fmt.Sprintf("attachment; filename=%s", filename)},
					"Cache-Control":       []string{"no-cache"},
				},
				Body: body,
			}

			return resp, nil
		}

		if req.fileType == "csv" {
			filename := fmt.Sprintf("cluster-%s-%s-%s.csv", req.ClusterID, req.from, req.until)
			metris := []UsageDataRecord{}
			err := json.Unmarshal(body, &metris)
			if err != nil {
				return nil, err
			}

			body, err := encodeUsageDataRecordToCSVBytes(metris)
			if err != nil {
				return nil, err
			}
			resp := RawResponse{
				Header: http.Header{
					"Content-Type":        []string{"txt/csv"},
					"Content-disposition": []string{fmt.Sprintf("attachment; filename=%s", filename)},
					"Cache-Control":       []string{"no-cache"},
				},
				Body: body,
			}

			return resp, nil
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

// returns csv encoded byte array to be used as response body
func encodeUsageDataRecordToCSVBytes(records []UsageDataRecord) ([]byte, error) {
	byteBuffer := &bytes.Buffer{}
	csvWriter := csv.NewWriter(byteBuffer)
	err := csvWriter.Write(csvColumns)

	if err != nil {
		return nil, err
	}
	for _, record := range records {
		var data []string
		data = append(data, record.Time)
		data = append(data, record.ClusterID)
		data = append(data, record.ProjectID)
		data = append(data, record.SeedClusterName)
		data = append(data, record.DatacenterName)
		data = append(data, record.ClusterOwner)
		data = append(data, record.CustomerProject)

		var cpu, memory, floatingIps int
		for _, node := range record.Metrics.Nodes {
			cpu += node.CPU
			memory += node.Memory
			floatingIps += node.FloatingIps
		}

		data = append(data, strconv.Itoa(cpu))
		data = append(data, strconv.Itoa(memory))
		data = append(data, strconv.Itoa(floatingIps))

		err := csvWriter.Write(data)

		if err != nil {
			return nil, err
		}
	}
	csvWriter.Flush()
	return byteBuffer.Bytes(), nil
}
