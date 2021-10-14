package openstack

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"k8c.io/kubermatic/v2/pkg/resources"
)

func getComputeClient(authURL, region string, credentials *resources.OpenstackCredentials, caBundle *x509.CertPool) (*gophercloud.ServiceClient, error) {
	authClient, err := getAuthClient(authURL, credentials, caBundle)
	if err != nil {
		return nil, err
	}

	computeClient, err := goopenstack.NewComputeV2(authClient, gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic, Region: region})
	// this is special case for  services that span only one region.
	//nolint:gosimple
	//lint:ignore S1020 false positive, we must do the errcheck regardless of if its an ErrEndpointNotFound
	if _, ok := err.(*gophercloud.ErrEndpointNotFound); ok {
		computeClient, err = goopenstack.NewComputeV2(authClient, gophercloud.EndpointOpts{})
		if err != nil {
			return nil, fmt.Errorf("couldn't get compute client: %v", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("couldn't get compute client: %v", err)
	}

	if computeClient != nil {
		// overwrite the default host/root CA Bundle with the proper CA Bundle
		computeClient.HTTPClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: caBundle}}
	}

	return computeClient, nil
}
