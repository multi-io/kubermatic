package openstack

import (
	oslimits "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/limits"
	osimages "github.com/gophercloud/gophercloud/openstack/compute/v2/images"
	osfloatingips "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
)

const (
	openstackFloatingIPErrorStatusName = "ERROR"
)

func GetImages(username, password, domain, tenant, tenantID, authURL, region string) ([]osimages.Image, error) {
	serviceClient, err := getComputeClient(username, password, domain, tenant, tenantID, authURL, region)
	if err != nil {
		return nil, err
	}

	allPages, err := osimages.ListDetail(serviceClient, osimages.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	allImages, err := osimages.ExtractImages(allPages)
	if err != nil {
		return nil, err
	}

	return allImages, nil
}

func GetQuotaLimits(username, password, domain, tenant, tenantID, authURL, region string) (*oslimits.Limits, error) {
	serviceClient, err := getComputeClient(username, password, domain, tenant, tenantID, authURL, region)
	if err != nil {
		return nil, err
	}

	limits, err := oslimits.Get(serviceClient, oslimits.GetOpts{}).Extract()
	if err != nil {
		return nil, err
	}

	return limits, nil
}

func GetUsedFloatingIPCount(username, password, domain, tenant, tenantID, authURL, region string) (int, error) {
	netClient, err := getNetClient(username, password, domain, tenant, tenantID, authURL, region)

	if err != nil {
		return 0, err
	}

	allPages, err := osfloatingips.List(netClient, osfloatingips.ListOpts{}).AllPages()
	if err != nil {
		return 0, err
	}

	allFIPs, err := osfloatingips.ExtractFloatingIPs(allPages)
	if err != nil {
		return 0, err
	}

	var usedIPCount = 0
	for _, f := range allFIPs {
		if f.Status != openstackFloatingIPErrorStatusName && f.PortID != "" {
			usedIPCount++
		}
	}

	return usedIPCount, nil
}

func GetFloatingIPQuota(username, password, domain, tenant, tenantID, authURL, region string) (int, error) {
	netClient, err := getNetClient(username, password, domain, tenant, tenantID, authURL, region)
	if err != nil {
		return 0, err
	}

	// tenantID is "" (comes from the client's request, which doesn't have it), we need the real tenant ID for this service
	realTenantID, err := GetCurrentTenantID(netClient).Extract()
	if err != nil {
		return 0, err
	}

	quotas, err := GetNeutronQuota(netClient, realTenantID).Extract()

	if err != nil {
		return 0, err
	}

	return quotas.FloatingIP, nil
}
