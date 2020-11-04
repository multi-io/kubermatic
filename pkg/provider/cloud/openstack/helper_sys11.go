package openstack

import (
	"fmt"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
)

func getComputeClient(username, password, domain, tenant, tenantID, authURL, region string) (*gophercloud.ServiceClient, error) {
	authClient, err := getAuthClient(username, password, domain, tenant, tenantID, authURL)
	if err != nil {
		return nil, err
	}

	computeClient, err := goopenstack.NewComputeV2(authClient, gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic, Region: region})
	// this is special case for  services that span only one region.
	if _, ok := err.(*gophercloud.ErrEndpointNotFound); ok {
		computeClient, err = goopenstack.NewComputeV2(authClient, gophercloud.EndpointOpts{})
		if err != nil {
			return nil, fmt.Errorf("couldn't get compute client: %v", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("couldn't get compute client: %v", err)
	}

	return computeClient, nil
}
