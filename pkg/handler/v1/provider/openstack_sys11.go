package provider

import (
	"context"
	"crypto/x509"
	"fmt"

	"k8c.io/kubermatic/v2/pkg/handler/middleware"
	k8cerrors "k8c.io/kubermatic/v2/pkg/util/errors"

	"k8c.io/kubermatic/v2/pkg/resources"

	"k8c.io/kubermatic/v2/pkg/handler/v1/common"

	"github.com/go-kit/kit/endpoint"
	handlercommon "k8c.io/kubermatic/v2/pkg/handler/common/provider"
	"k8c.io/kubermatic/v2/pkg/provider"
	"k8c.io/kubermatic/v2/pkg/provider/cloud/openstack"
)

func OpenstackImageEndpoint(seedsGetter provider.SeedsGetter, presetsProvider provider.PresetProvider, userInfoGetter provider.UserInfoGetter, caBundle *x509.CertPool) endpoint.Endpoint {
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

		t := ctx.Value(middleware.RawTokenContextKey)
		token, ok := t.(string)
		if !ok || token == "" {
			return nil, k8cerrors.NewNotAuthorized()
		}

		var cred *resources.OpenstackCredentials
		if req.Credential == "" {
			cred = &resources.OpenstackCredentials{
				Username:                    req.Username,
				Password:                    req.Password,
				Tenant:                      req.Tenant,
				TenantID:                    req.TenantID,
				Domain:                      req.Domain,
				ApplicationCredentialID:     req.ApplicationCredentialID,
				ApplicationCredentialSecret: req.ApplicationCredentialSecret,
				Token:                       token,
			}
		} else {
			cred, err = getPresetCredentials(userInfo, req.Credential, presetsProvider, token)
			if err != nil {
				return nil, fmt.Errorf("error getting preset credentials for OpenStack: %v", err)
			}
		}
		return openstack.GetImages(datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region, cred, caBundle)
	}
}

func OpenstackQuotaLimitEndpoint(seedsGetter provider.SeedsGetter, presetsProvider provider.PresetProvider, userInfoGetter provider.UserInfoGetter, caBundle *x509.CertPool) endpoint.Endpoint {
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

		t := ctx.Value(middleware.RawTokenContextKey)
		token, ok := t.(string)
		if !ok || token == "" {
			return nil, k8cerrors.NewNotAuthorized()
		}
		var cred *resources.OpenstackCredentials
		if req.Credential == "" {
			cred = &resources.OpenstackCredentials{
				Username:                    req.Username,
				Password:                    req.Password,
				Tenant:                      req.Tenant,
				TenantID:                    req.TenantID,
				Domain:                      req.Domain,
				ApplicationCredentialID:     req.ApplicationCredentialID,
				ApplicationCredentialSecret: req.ApplicationCredentialSecret,
				Token:                       token,
			}
		} else {
			cred, err = getPresetCredentials(userInfo, req.Credential, presetsProvider, token)
			if err != nil {
				return nil, fmt.Errorf("error getting preset credentials for OpenStack: %v", err)
			}
		}

		return handlercommon.GetOpenstackQuotaLimits(datacenter.Spec.Openstack.AuthURL, datacenter.Spec.Openstack.Region, cred, caBundle)
	}
}

func OpenstackImageNoCredentialsEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter, caBundle *x509.CertPool) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(OpenstackNoCredentialsReq)
		return handlercommon.OpenstackImageNoCredentialsEndpoint(ctx, req.ProjectID, req.ClusterID, projectProvider, privilegedProjectProvider, seedsGetter, userInfoGetter, caBundle)
	}
}

func OpenstackQuotaLimitNoCredentialsEndpoint(projectProvider provider.ProjectProvider, privilegedProjectProvider provider.PrivilegedProjectProvider, seedsGetter provider.SeedsGetter, userInfoGetter provider.UserInfoGetter, caBundle *x509.CertPool) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(OpenstackNoCredentialsReq)
		return handlercommon.OpenstackQuotaLimitNoCredentialsEndpoint(ctx, req.ProjectID, req.ClusterID, projectProvider, privilegedProjectProvider, seedsGetter, userInfoGetter, caBundle)
	}
}

func getOpenstackCredentials(userInfo *provider.UserInfo, credentialName, username, password, domain, tenant, tenantID string, presetProvider provider.PresetProvider) (string, string, string, string, string, error) {
	if len(credentialName) > 0 {
		preset, err := presetProvider.GetPreset(userInfo, credentialName)
		if err != nil {
			return "", "", "", "", "", fmt.Errorf("can not get preset %s for the user %s", credentialName, userInfo.Email)
		}
		if credentials := preset.Spec.Openstack; credentials != nil {
			username = credentials.Username
			password = credentials.Password
			tenant = credentials.Tenant
			tenantID = credentials.TenantID
			domain = credentials.Domain
		}
	}
	return username, password, domain, tenant, tenantID, nil
}
