package provider

import (
	"context"
	"fmt"

	"k8c.io/kubermatic/v2/pkg/handler/v1/common"

	"github.com/go-kit/kit/endpoint"
	handlercommon "k8c.io/kubermatic/v2/pkg/handler/common/provider"
	"k8c.io/kubermatic/v2/pkg/provider"
	"k8c.io/kubermatic/v2/pkg/provider/cloud/openstack"
)

func OpenstackImageEndpoint(seedsGetter provider.SeedsGetter, presetsProvider provider.PresetProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req, ok := request.(OpenstackReq)
		if !ok {
			return nil, fmt.Errorf("incorrect type of request, expected = OpenstackReq, got = %T", request)
		}
		userInfo, err := userInfoGetter(ctx, "")
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		datacenterName := req.DatacenterName
		_, datacenter, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, datacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		username, password, domain, tenant, tenantID, err := getOpenstackCredentials(userInfo, req.Credential, req.Username, req.Password, req.Domain, req.Tenant, req.TenantID, presetsProvider)
		if err != nil {
			return nil, fmt.Errorf("error getting OpenStack credentials: %v", err)
		}

		return openstack.GetImages(username, password, domain, tenant, tenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region)
	}
}

func OpenstackQuotaLimitEndpoint(seedsGetter provider.SeedsGetter, presetsProvider provider.PresetProvider, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req, ok := request.(OpenstackReq)
		if !ok {
			return nil, fmt.Errorf("incorrect type of request, expected = OpenstackReq, got = %T", request)
		}
		userInfo, err := userInfoGetter(ctx, "")
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		datacenterName := req.DatacenterName
		_, datacenter, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, datacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		username, password, domain, tenant, tenantID, err := getOpenstackCredentials(userInfo, req.Credential, req.Username, req.Password, req.Domain, req.Tenant, req.TenantID, presetsProvider)
		if err != nil {
			return nil, fmt.Errorf("error getting OpenStack credentials: %v", err)
		}

		return handlercommon.GetOpenstackQuotaLimits(username, password, domain, tenant, tenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region)
	}
}

func OpenstackImageNoCredentialsEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(OpenstackNoCredentialsReq)
		return handlercommon.OpenstackImageNoCredentialsEndpoint(ctx, req.ProjectID, req.ClusterID, projectProvider, privilegedProjectProvider, seedsGetter, userInfoGetter)
	}
}

func OpenstackQuotaLimitNoCredentialsEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(OpenstackNoCredentialsReq)
		return handlercommon.OpenstackQuotaLimitNoCredentialsEndpoint(ctx, req.ProjectID, req.ClusterID, projectProvider, privilegedProjectProvider, seedsGetter, userInfoGetter)
	}
}
