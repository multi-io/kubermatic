package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kubermatic/kubermatic/pkg/resources"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/go-kit/kit/endpoint"

	"github.com/kubermatic/kubermatic/pkg/handler/middleware"
	"github.com/kubermatic/kubermatic/pkg/handler/v1/common"
	"github.com/kubermatic/kubermatic/pkg/log"
	"github.com/kubermatic/kubermatic/pkg/provider"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// unsealVaultReq defines HTTP request for unsealVaultAddon
// swagger:parameters unsealVaultAddon
type unsealVaultReq struct {
	common.GetClusterReq
	// in: body
	Body unsealKeys
}

// swagger:model UnsealKeys
type unsealKeys struct {
	Keys []string `json:"keys"`
}

type RawResponse struct {
	Header http.Header
	Body   []byte
}

func DecodeUnsealVaultAddon(c context.Context, r *http.Request) (interface{}, error) {
	var req unsealVaultReq

	cr, err := common.DecodeGetClusterReq(c, r)
	if err != nil {
		return nil, err
	}

	req.GetClusterReq = cr.(common.GetClusterReq)

	if err := json.NewDecoder(r.Body).Decode(&req.Body); err != nil {
		return nil, err
	}

	return req, nil
}

type UnsealResponse struct {
	Unseals int `json:"unseals"`
}

type UnsealRequest struct {
	Key string `json:"key"`
}

func UnsealVaultAddonEndpoint(seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		req := request.(unsealVaultReq)
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

		seedConfigSecret, err := masterKubeClient.CoreV1().Secrets("kubermatic").Get("kubeconfig", metaV1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig secret: %v", err)
		}

		seedConfig, err := getKubeconfig(seed.Name, seedConfigSecret.Data[resources.KubeconfigSecretKey])
		if err != nil {
			return nil, fmt.Errorf("failed to get seed kubeconfig: %v", err)
		}

		seedKubeClient, err := kubernetes.NewForConfig(seedConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create a seed kubernetes client: %v", err)
		}

		clusterKubeConfigSecret, err := seedKubeClient.CoreV1().Secrets(fmt.Sprintf("cluster-%s", c.Name)).Get("admin-kubeconfig", metaV1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster kubeconfig secret: %v", err)
		}

		clusterKubeConfig, err := getKubeconfig(resources.KubeconfigDefaultContextKey, clusterKubeConfigSecret.Data[resources.KubeconfigSecretKey])
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster kubeconfig: %v", err)
		}

		clusterKubeClient, err := kubernetes.NewForConfig(clusterKubeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create a cluster kubernetes client: %v", err)
		}

		// find metakube-vault addon pods
		requirement, err := labels.NewRequirement("app.kubernetes.io/instance", selection.Equals, []string{"syseleven-vault"})
		if err != nil {
			return nil, fmt.Errorf("failed to build label selector: %v", err)
		}
		options := metaV1.ListOptions{
			LabelSelector: requirement.String(),
		}
		endpoints, err := clusterKubeClient.CoreV1().Pods("syseleven-vault").List(options)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch vault pods: %v", err)
		}

		unsealResponse := &UnsealResponse{
			Unseals: 0,
		}

		for _, pod := range endpoints.Items {
			for _, key := range req.Body.Keys {
				requestBody, _ := json.Marshal(UnsealRequest{
					Key: key,
				})
				proxyRequest := clusterKubeClient.CoreV1().RESTClient().Put().
					Namespace(pod.Namespace).
					Resource("pods").
					Name(pod.Name).
					SubResource("proxy").
					Suffix("v1/sys/unseal").
					Body(requestBody)
				_, err := proxyRequest.DoRaw()
				if err != nil {
					// only log errors, so that one already unsealed pod does not break the whole unsealing process
					log.Logger.Error(err)
				}
				unsealResponse.Unseals++
			}
		}

		body, _ := json.Marshal(unsealResponse)
		resp := RawResponse{
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: body,
		}

		return resp, nil
	}
}

func getKubeconfig(contextName string, cfg []byte) (*rest.Config, error) {
	clusterClientCfg, err := clientcmd.Load(cfg)
	if err != nil {
		return nil, err
	}

	return clientcmd.NewNonInteractiveClientConfig(
		*clusterClientCfg,
		contextName,
		&clientcmd.ConfigOverrides{},
		nil,
	).ClientConfig()
}
