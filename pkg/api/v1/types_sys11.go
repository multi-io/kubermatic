package v1

import (
	oslimits "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/limits"
)

// Image represents an Image returned by the Compute API.
// swagger:model Image
type Image struct {
	// ID is the unique ID of an image.
	ID string

	// Created is the date when the image was created.
	Created string

	// MinDisk is the minimum amount of disk a flavor must have to be able
	// to create a server based on the image, measured in GB.
	MinDisk int

	// MinRAM is the minimum amount of RAM a flavor must have to be able
	// to create a server based on the image, measured in MB.
	MinRAM int

	// Name provides a human-readable moniker for the OS image.
	Name string

	// The Progress and Status fields indicate image-creation status.
	Progress int

	// Status is the current status of the image.
	Status string

	// Update is the date when the image was updated.
	Updated string

	// Metadata provides free-form key/value pairs that further describe the
	// image.
	Metadata map[string]interface{}
}

// Quotas is a struct that contains the response of a Quotas query.
// swagger:model Quotas
type Quotas struct {
	// Limits contains the limits and usage information.
	Limits *oslimits.Limits `json:"limits"`

	// UsedFloatingIpCount is the floating IP quota
	UsedFloatingIPCount int `json:"usedFloatingIpCount"`

	// FloatingIpQuota Sys11 addition with the amount of used and attached floating ips
	FloatingIPQuota int `json:"floatingIpQuota"`
}

// Limits is a struct that contains the response of a limit query.
// swagger:model Limits
type Limits struct {
	// Absolute contains the limits and usage information.
	Absolute Absolute `json:"absolute"`
}

// Usage is a struct that contains the current resource usage and limits
// of a tenant.
type Absolute struct {
	// MaxTotalCores is the number of cores available to a tenant.
	MaxTotalCores int `json:"maxTotalCores"`

	// MaxImageMeta is the amount of image metadata available to a tenant.
	MaxImageMeta int `json:"maxImageMeta"`

	// MaxServerMeta is the amount of server metadata available to a tenant.
	MaxServerMeta int `json:"maxServerMeta"`

	// MaxPersonality is the amount of personality/files available to a tenant.
	MaxPersonality int `json:"maxPersonality"`

	// MaxPersonalitySize is the personality file size available to a tenant.
	MaxPersonalitySize int `json:"maxPersonalitySize"`

	// MaxTotalKeypairs is the total keypairs available to a tenant.
	MaxTotalKeypairs int `json:"maxTotalKeypairs"`

	// MaxSecurityGroups is the number of security groups available to a tenant.
	MaxSecurityGroups int `json:"maxSecurityGroups"`

	// MaxSecurityGroupRules is the number of security group rules available to
	// a tenant.
	MaxSecurityGroupRules int `json:"maxSecurityGroupRules"`

	// MaxServerGroups is the number of server groups available to a tenant.
	MaxServerGroups int `json:"maxServerGroups"`

	// MaxServerGroupMembers is the number of server group members available
	// to a tenant.
	MaxServerGroupMembers int `json:"maxServerGroupMembers"`

	// MaxTotalFloatingIps is the number of floating IPs available to a tenant.
	MaxTotalFloatingIps int `json:"maxTotalFloatingIps"`

	// MaxTotalInstances is the number of instances/servers available to a tenant.
	MaxTotalInstances int `json:"maxTotalInstances"`

	// MaxTotalRAMSize is the total amount of RAM available to a tenant measured
	// in megabytes (MB).
	MaxTotalRAMSize int `json:"maxTotalRAMSize"`

	// TotalCoresUsed is the number of cores currently in use.
	TotalCoresUsed int `json:"totalCoresUsed"`

	// TotalInstancesUsed is the number of instances/servers in use.
	TotalInstancesUsed int `json:"totalInstancesUsed"`

	// TotalFloatingIpsUsed is the number of floating IPs in use.
	TotalFloatingIpsUsed int `json:"totalFloatingIpsUsed"`

	// TotalRAMUsed is the total RAM/memory in use measured in megabytes (MB).
	TotalRAMUsed int `json:"totalRAMUsed"`

	// TotalSecurityGroupsUsed is the total number of security groups in use.
	TotalSecurityGroupsUsed int `json:"totalSecurityGroupsUsed"`

	// TotalServerGroupsUsed is the total number of server groups in use.
	TotalServerGroupsUsed int `json:"totalServerGroupsUsed"`
}

// NodeDeploymentRequest represents an asynchronous request to create a NodeDeployment in a user cluster
// swagger:model NodeDeploymentRequest
type NodeDeploymentRequest struct {
	ObjectMeta `json:",inline"`

	Spec NodeDeploymentRequestSpec `json:"spec"`
}

// NodeDeploymentRequestSpec node deployment request specification
// swagger:model NodeDeploymentRequestSpec
type NodeDeploymentRequestSpec struct {
	Nd NodeDeployment `json:"nd"`
}
