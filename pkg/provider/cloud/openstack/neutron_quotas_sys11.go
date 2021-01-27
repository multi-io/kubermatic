package openstack

import (
	"github.com/gophercloud/gophercloud"
)

const resourcePath = "quotas"

func GetNeutronQuota(client *gophercloud.ServiceClient, tenantID string) (r GetNeutronQuotaResult) {
	url := client.ServiceURL(resourcePath, tenantID)

	resp, err := client.Get(url, &r.Body, nil)
	r.Err = err
	if err != nil {
		return
	}
	defer resp.Body.Close()

	return
}

func GetCurrentTenantID(client *gophercloud.ServiceClient) (r GetCurrentTenantResult) {
	url := client.ServiceURL(resourcePath, "tenant")

	resp, err := client.Get(url, &r.Body, nil)
	r.Err = err
	if err != nil {
		return
	}
	defer resp.Body.Close()
	return
}

type GetNeutronQuotaResult struct {
	gophercloud.Result
}

func (r GetNeutronQuotaResult) Extract() (*Quotas, error) {
	var s struct {
		Quotas *Quotas `json:"quota"`
	}
	err := r.ExtractInto(&s)
	return s.Quotas, err
}

type GetCurrentTenantResult struct {
	gophercloud.Result
}

func (r GetCurrentTenantResult) Extract() (string, error) {
	var s struct {
		Tenant struct {
			TenantID string `json:"tenant_id"`
		} `json:"tenant"`
	}
	err := r.ExtractInto(&s)
	return s.Tenant.TenantID, err
}

type Quotas struct {
	Firewall int `json:"firewall"`

	Member int `json:"member"`

	Port int `json:"port"`

	Subnet int `json:"subnet"`

	FirewallRule int `json:"firewall_rule"`

	Network int `json:"network"`

	IpsecSiteConnection int `json:"ipsec_site_connection"`

	FloatingIP int `json:"floatingip"`

	Graph int `json:"graph"`

	SecurityGroupRule int `json:"security_group_rule"`

	RbacPolicy int `json:"rbac_policy"`

	EndpointGroup int `json:"endpoint_group"`

	IkePolicy int `json:"ikepolicy"`

	IpsecPolicy int `json:"ipsecpolicy"`

	L7Policy int `json:"l7policy"`

	Subnetpool int `json:"subnetpool"`

	Listener int `json:"listener"`

	Pool int `json:"pool"`

	FirewallPolicy int `json:"firewall_policy"`

	VpnService int `json:"vpnservice"`

	HealhMonitor int `json:"healthmonitor"`

	SecurityGroup int `json:"security_group"`

	Router int `json:"router"`

	LoadBalancer int `json:"loadbalancer"`
}
