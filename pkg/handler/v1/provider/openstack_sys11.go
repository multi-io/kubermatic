package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kubermatic/kubermatic/pkg/handler/middleware"
	"github.com/kubermatic/kubermatic/pkg/handler/v1/common"

	v1 "github.com/kubermatic/kubermatic/pkg/api/v1"

	"github.com/go-kit/kit/endpoint"
	"github.com/kubermatic/kubermatic/pkg/provider"
	"github.com/kubermatic/kubermatic/pkg/provider/cloud/openstack"
	kubernetesprovider "github.com/kubermatic/kubermatic/pkg/provider/kubernetes"
	"github.com/kubermatic/kubermatic/pkg/util/errors"
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

		return getOpenstackQuotaLimits(username, password, domain, tenant, tenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region)
	}
}

func OpenstackImageNoCredentialsEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(OpenstackNoCredentialsReq)
		cluster, err := getClusterForOpenstack(ctx, projectProvider, privilegedProjectProvider, userInfoGetter, req.ProjectID, req.ClusterID)
		if err != nil {
			return nil, err
		}

		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		datacenterName := cluster.Spec.Cloud.DatacenterName
		_, datacenter, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, datacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		assertedClusterProvider, ok := clusterProvider.(*kubernetesprovider.ClusterProvider)
		if !ok {
			return nil, errors.New(http.StatusInternalServerError, "failed to assert clusterProvider")
		}

		secretKeySelector := provider.SecretKeySelectorValueFuncFactory(ctx, assertedClusterProvider.GetSeedClusterAdminRuntimeClient())
		creds, err := openstack.GetCredentialsForCluster(cluster.Spec.Cloud, secretKeySelector)
		if err != nil {
			return nil, err
		}

		return openstack.GetImages(creds.Username, creds.Password, creds.Domain, creds.Tenant, creds.TenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region)
	}
}

func OpenstackQuotaLimitNoCredentialsEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(OpenstackNoCredentialsReq)
		cluster, err := getClusterForOpenstack(ctx, projectProvider, privilegedProjectProvider, userInfoGetter, req.ProjectID, req.ClusterID)
		if err != nil {
			return nil, err
		}

		userInfo, err := userInfoGetter(ctx, req.ProjectID)
		if err != nil {
			return nil, common.KubernetesErrorToHTTPError(err)
		}

		datacenterName := cluster.Spec.Cloud.DatacenterName
		_, datacenter, err := provider.DatacenterFromSeedMap(userInfo, seedsGetter, datacenterName)
		if err != nil {
			return nil, fmt.Errorf("error getting dc: %v", err)
		}

		clusterProvider := ctx.Value(middleware.ClusterProviderContextKey).(provider.ClusterProvider)
		assertedClusterProvider, ok := clusterProvider.(*kubernetesprovider.ClusterProvider)
		if !ok {
			return nil, errors.New(http.StatusInternalServerError, "failed to assert clusterProvider")
		}

		secretKeySelector := provider.SecretKeySelectorValueFuncFactory(ctx, assertedClusterProvider.GetSeedClusterAdminRuntimeClient())
		creds, err := openstack.GetCredentialsForCluster(cluster.Spec.Cloud, secretKeySelector)
		if err != nil {
			return nil, err
		}

		return getOpenstackQuotaLimits(creds.Username, creds.Password, creds.Domain, creds.Tenant, creds.TenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region)
	}
}

func getOpenstackQuotaLimits(username, password, domain, tenant, tenantID, authURL, region string) (*v1.Quotas, error) {
	limits, err := openstack.GetQuotaLimits(username, password, domain, tenant, tenantID, authURL, region)
	if err != nil {
		return nil, err
	}

	usedFloatingIPCount, err := openstack.GetUsedFloatingIPCount(username, password, domain, tenant, tenantID, authURL, region)
	if err != nil {
		return nil, err
	}

	floatingIPQuota, err := openstack.GetFloatingIPQuota(username, password, domain, tenant, tenantID, authURL, region)
	if err != nil {
		return nil, err
	}

	apiLimits := &v1.Quotas{
		Limits:              limits,
		UsedFloatingIPCount: usedFloatingIPCount,
		FloatingIPQuota:     floatingIPQuota,
	}

	return apiLimits, nil
}
