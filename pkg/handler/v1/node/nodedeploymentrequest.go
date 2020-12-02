package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/Masterminds/semver/v3"
	jsonpatch "github.com/evanphx/json-patch"
	"k8c.io/kubermatic/v2/pkg/validation/nodeupdate"

	"github.com/go-kit/kit/endpoint"
	"github.com/gorilla/mux"

	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"
	"k8c.io/kubermatic/v2/pkg/provider"
	kubernetesprovider "k8c.io/kubermatic/v2/pkg/provider/kubernetes"
	machineresource "k8c.io/kubermatic/v2/pkg/resources/machine"
	k8cerrors "k8c.io/kubermatic/v2/pkg/util/errors"
)

// ndrReq defines HTTP request for getNodeDeploymentRequest and deleteNodeDeploymentRequest
// swagger:parameters getNodeDeploymentRequest deleteNodeDeploymentRequest
type ndrReq struct {
	common.GetClusterReq
	// in: path
	NdrName string `json:"ndrequest_id"`
}

// listReq defines HTTP request for listNodeDeploymentRequests endpoint
// swagger:parameters listNodeDeploymentRequests
type listReq struct {
	common.GetClusterReq
}

// createReq defines HTTP request for createNodeDeploymentRequest endpoint
// swagger:parameters createNodeDeploymentRequest
type createReq struct {
	common.GetClusterReq
	// in: body
	Body apiv1.NodeDeploymentRequest
}

// patchReq defines HTTP request for patchNodeDeploymentRequest endpoint
// swagger:parameters patchNodeDeploymentRequest
type patchReq struct {
	ndrReq
	// in: body
	Patch []byte
}

func DecodeGetNodeDeploymentRequest(c context.Context, r *http.Request) (interface{}, error) {
	var req ndrReq

	cr, err := common.DecodeGetClusterReq(c, r)
	if err != nil {
		return nil, err
	}

	ndrName, err := decodeNdrName(c, r)
	if err != nil {
		return nil, err
	}

	req.GetClusterReq = cr.(common.GetClusterReq)
	req.NdrName = ndrName

	return req, nil
}

func DecodeListNodeDeploymentRequests(c context.Context, r *http.Request) (interface{}, error) {
	var req listReq

	cr, err := common.DecodeGetClusterReq(c, r)
	if err != nil {
		return nil, err
	}

	req.GetClusterReq = cr.(common.GetClusterReq)

	return req, nil
}

func DecodeCreateNodeDeploymentRequest(c context.Context, r *http.Request) (interface{}, error) {
	var req createReq

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

func DecodePatchNodeDeploymentRequest(c context.Context, r *http.Request) (interface{}, error) {
	var req patchReq

	gr, err := DecodeGetNodeDeploymentRequest(c, r)
	if err != nil {
		return nil, err
	}

	req.ndrReq = gr.(ndrReq)

	if req.Patch, err = ioutil.ReadAll(r.Body); err != nil {
		return nil, err
	}

	return req, nil
}

func decodeNdrName(c context.Context, r *http.Request) (string, error) {
	ndrName := mux.Vars(r)["ndrequest_id"]
	if ndrName == "" {
		return "", fmt.Errorf("'ndrequest_id' parameter is required but was not provided")
	}

	return ndrName, nil
}

func GetNodeDeploymentRequestEndpoint(projectProvider provider.ProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(ndrReq)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		_, err = projectProvider.Get(userInfo, req.ProjectID, &provider.ProjectGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		cluster, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		mdrProvider := ctx.Value(middleware.MachineDeploymentRequestProviderContextKey).(provider.MachineDeploymentRequestProvider)
		mdr, err := mdrProvider.Get(userInfo, cluster, req.NdrName)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		result, err := convertMdrToNdr(mdr)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		return result, nil
	}
}

func ListNodeDeploymentRequestEndpoint(projectProvider provider.ProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(listReq)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		_, err = projectProvider.Get(userInfo, req.ProjectID, &provider.ProjectGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		cluster, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		mdrProvider := ctx.Value(middleware.MachineDeploymentRequestProviderContextKey).(provider.MachineDeploymentRequestProvider)
		mdrs, err := mdrProvider.List(userInfo, cluster)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		result, err := convertMdrToNdrList(mdrs)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		return result, nil
	}
}

func CreateNodeDeploymentRequestEndpoint(sshKeyProvider provider.SSHKeyProvider, projectProvider provider.ProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(createReq)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		project, err := projectProvider.Get(userInfo, req.ProjectID, &provider.ProjectGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		cluster, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		assertedClusterProvider, ok := clusterProvider.(*kubernetesprovider.ClusterProvider)
		if !ok {
			return nil, k8cerrors.New(http.StatusInternalServerError, "clusterprovider is not a kubernetesprovider.Clusterprovider, can not create secret")
		}

		data := common.CredentialsData{
			KubermaticCluster: cluster,
			Client:            assertedClusterProvider.GetSeedClusterAdminRuntimeClient(),
		}

		keys, err := sshKeyProvider.List(project, &provider.SSHKeyListOptions{ClusterName: req.ClusterID})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		_, dc, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, cluster.Spec.Cloud.DatacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		nd, err := machineresource.Validate(&req.Body.Spec.Nd, cluster.Spec.Version.Semver())
		if err != nil {
			return nil, k8cerrors.NewBadRequest(fmt.Sprintf("node deployment validation failed: %s", err.Error()))
		}

		md, err := machineresource.Deployment(cluster, nd, dc, keys, data)
		if err != nil {
			return nil, fmt.Errorf("failed to create machine deployment from template: %v", err)
		}

		mdrProvider := ctx.Value(middleware.MachineDeploymentRequestProviderContextKey).(provider.MachineDeploymentRequestProvider)
		mdr, err := mdrProvider.New(userInfo, cluster, md.Name, md)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		result, err := convertMdrToNdr(mdr)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		return result, nil
	}
}

func PatchNodeDeploymentRequestEndpoint(sshKeyProvider provider.SSHKeyProvider, projectProvider provider.ProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(patchReq)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		project, err := projectProvider.Get(userInfo, req.ProjectID, &provider.ProjectGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		cluster, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		mdrProvider := ctx.Value(middleware.MachineDeploymentRequestProviderContextKey).(provider.MachineDeploymentRequestProvider)
		mdr, err := mdrProvider.Get(userInfo, cluster, req.NdrName)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		ndr, err := convertMdrToNdr(mdr)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		ndrJSON, err := json.Marshal(ndr)
		if err != nil {
			return nil, fmt.Errorf("cannot decode existing node deployment request: %v", err)
		}

		patchedNdrJSON, err := jsonpatch.MergePatch(ndrJSON, req.Patch)
		if err != nil {
			return nil, fmt.Errorf("cannot patch node deployment request: %v", err)
		}

		var patchedNdr *apiv1.NodeDeploymentRequest
		if err := json.Unmarshal(patchedNdrJSON, &patchedNdr); err != nil {
			return nil, fmt.Errorf("cannot decode patched cluster: %v", err)
		}

		// version validation as in node.go / PatchNodeDeployment
		kversion, err := semver.NewVersion(patchedNdr.Spec.Nd.Spec.Template.Versions.Kubelet)
		if err != nil {
			return nil, k8cerrors.NewBadRequest("failed to parse kubelet version: %v", err)
		}
		if err = nodeupdate.EnsureVersionCompatible(cluster.Spec.Version.Semver(), kversion); err != nil {
			return nil, k8cerrors.NewBadRequest(err.Error())
		}

		assertedClusterProvider, ok := clusterProvider.(*kubernetesprovider.ClusterProvider)
		if !ok {
			return nil, k8cerrors.New(http.StatusInternalServerError, "clusterprovider is not a kubernetesprovider.Clusterprovider, can not create secret")
		}

		data := common.CredentialsData{
			KubermaticCluster: cluster,
			Client:            assertedClusterProvider.GetSeedClusterAdminRuntimeClient(),
		}

		keys, err := sshKeyProvider.List(project, &provider.SSHKeyListOptions{ClusterName: req.ClusterID})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		_, dc, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, cluster.Spec.Cloud.DatacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		patchedMd, err := machineresource.Deployment(cluster, &patchedNdr.Spec.Nd, dc, keys, data)
		if err != nil {
			return nil, fmt.Errorf("failed to create machine deployment from template: %v", err)
		}

		mdr.Spec.Md = *patchedMd

		_, err = mdrProvider.Update(userInfo, cluster, mdr)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		return patchedNdr, nil
	}
}

func DeleteNodeDeploymentRequestEndpoint(projectProvider provider.ProjectProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(ndrReq)
		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		_, err = projectProvider.Get(userInfo, req.ProjectID, &provider.ProjectGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}
		cluster, err := clusterProvider.Get(userInfo, req.ClusterID, &provider.ClusterGetOptions{})
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		mdrProvider := ctx.Value(middleware.MachineDeploymentRequestProviderContextKey).(provider.MachineDeploymentRequestProvider)

		return nil, common.KubernetesErrorToHTTPError(mdrProvider.Delete(userInfo, cluster, req.NdrName))
	}
}

func convertMdrToNdr(mdr *kubermaticv1.MachineDeploymentRequest) (*apiv1.NodeDeploymentRequest, error) {
	nd, err := outputMachineDeployment(&mdr.Spec.Md)
	if err != nil {
		return nil, err
	}

	result := &apiv1.NodeDeploymentRequest{
		ObjectMeta: apiv1.ObjectMeta{
			ID:                mdr.Name,
			Name:              mdr.Name,
			CreationTimestamp: apiv1.NewTime(mdr.CreationTimestamp.Time),
			DeletionTimestamp: func() *apiv1.Time {
				if mdr.DeletionTimestamp != nil {
					deletionTimestamp := apiv1.NewTime(mdr.DeletionTimestamp.Time)
					return &deletionTimestamp
				}
				return nil
			}(),
		},
		Spec: apiv1.NodeDeploymentRequestSpec{
			Nd: *nd,
		},
	}
	return result, nil
}

func convertMdrToNdrList(mdrs []*kubermaticv1.MachineDeploymentRequest) ([]*apiv1.NodeDeploymentRequest, error) {
	result := []*apiv1.NodeDeploymentRequest{}

	for _, mdr := range mdrs {
		converted, err := convertMdrToNdr(mdr)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}

	return result, nil
}
