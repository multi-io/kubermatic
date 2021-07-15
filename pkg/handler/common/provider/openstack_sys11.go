package provider

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"

	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"

	v1 "k8c.io/kubermatic/v2/pkg/api/v1"

	"k8c.io/kubermatic/v2/pkg/provider"
	"k8c.io/kubermatic/v2/pkg/provider/cloud/openstack"
	kubernetesprovider "k8c.io/kubermatic/v2/pkg/provider/kubernetes"
	"k8c.io/kubermatic/v2/pkg/util/errors"
)

func OpenstackImageNoCredentialsEndpoint(ctx context.Context, projectID, clusterID string, projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter, caBundle *x509.CertPool) (interface{}, error) {
	cluster, err := getClusterForOpenstack(ctx, projectProvider, privilegedProjectProvider, userInfoGetter, projectID, clusterID)
	if err != nil {
		return nil, err
	}

	userInfo, err := userInfoGetter(ctx, projectID)
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

	return openstack.GetImages(creds.Username, creds.Password, creds.Domain, creds.Tenant, creds.TenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region, caBundle)
}

func OpenstackQuotaLimitNoCredentialsEndpoint(ctx context.Context, projectID, clusterID string, projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter, caBundle *x509.CertPool) (interface{}, error) {
	cluster, err := getClusterForOpenstack(ctx, projectProvider, privilegedProjectProvider, userInfoGetter, projectID, clusterID)
	if err != nil {
		return nil, err
	}

	userInfo, err := userInfoGetter(ctx, projectID)
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

	return GetOpenstackQuotaLimits(creds.Username, creds.Password, creds.Domain, creds.Tenant, creds.TenantID, datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region, caBundle)
}

func GetOpenstackQuotaLimits(username, password, domain, tenant, tenantID, authURL, region string, caBundle *x509.CertPool) (*v1.Quotas, error) {
	limits, err := openstack.GetQuotaLimits(username, password, domain, tenant, tenantID, authURL, region, caBundle)
	if err != nil {
		return nil, err
	}

	usedFloatingIPCount, err := openstack.GetUsedFloatingIPCount(username, password, domain, tenant, tenantID, authURL, region, caBundle)
	if err != nil {
		return nil, err
	}

	floatingIPQuota, err := openstack.GetFloatingIPQuota(username, password, domain, tenant, tenantID, authURL, region, caBundle)
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
