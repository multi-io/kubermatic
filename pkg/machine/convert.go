/*
Copyright 2020 The Kubermatic Kubernetes Platform contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package machine

import (
	"fmt"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	alibaba "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/alibaba/types"
	anexia "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	aws "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws/types"
	azure "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/azure/types"
	digitalocean "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/digitalocean/types"
	gce "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/gce/types"
	hetzner "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/hetzner/types"
	kubevirt "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/kubevirt/types"
	openstack "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack/types"
	packet "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/packet/types"
	vsphere "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vsphere/types"
	providerconfig "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"
	"github.com/kubermatic/machine-controller/pkg/userdata/rhel"
	"github.com/kubermatic/machine-controller/pkg/userdata/sles"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"
	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"

	"k8s.io/apimachinery/pkg/util/json"
)

// GetAPIV1OperatingSystemSpec returns the api compatible OperatingSystemSpec for the given machine
func GetAPIV1OperatingSystemSpec(machineSpec clusterv1alpha1.MachineSpec) (*apiv1.OperatingSystemSpec, error) {
	decodedProviderSpec, err := providerconfig.GetConfig(machineSpec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine providerConfig: %v", err)
	}

	operatingSystemSpec := &apiv1.OperatingSystemSpec{}

	switch decodedProviderSpec.OperatingSystem {
	case providerconfig.OperatingSystemFlatcar:
		config := &flatcar.Config{}
		if err := json.Unmarshal(decodedProviderSpec.OperatingSystemSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse flatcar config: %v", err)
		}

		operatingSystemSpec.Flatcar = &apiv1.FlatcarSpec{
			DisableAutoUpdate: config.DisableAutoUpdate,
		}
		if config.ProvisioningUtility == flatcar.CloudInit {
			operatingSystemSpec.Flatcar.ProvisioningUtility = flatcar.CloudInit
		}

	case providerconfig.OperatingSystemUbuntu:
		config := &ubuntu.Config{}
		if err := json.Unmarshal(decodedProviderSpec.OperatingSystemSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse ubuntu config: %v", err)
		}
		operatingSystemSpec.Ubuntu = &apiv1.UbuntuSpec{
			DistUpgradeOnBoot: config.DistUpgradeOnBoot,
		}

	case providerconfig.OperatingSystemCentOS:
		config := &centos.Config{}
		if err := json.Unmarshal(decodedProviderSpec.OperatingSystemSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse centos config: %v", err)
		}
		operatingSystemSpec.CentOS = &apiv1.CentOSSpec{
			DistUpgradeOnBoot: config.DistUpgradeOnBoot,
		}

	case providerconfig.OperatingSystemSLES:
		config := &sles.Config{}
		if err := json.Unmarshal(decodedProviderSpec.OperatingSystemSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse sles config: %v", err)
		}
		operatingSystemSpec.SLES = &apiv1.SLESSpec{
			DistUpgradeOnBoot: config.DistUpgradeOnBoot,
		}

	case providerconfig.OperatingSystemRHEL:
		config := &rhel.Config{}
		if err := json.Unmarshal(decodedProviderSpec.OperatingSystemSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse rhel config: %v", err)
		}
		operatingSystemSpec.RHEL = &apiv1.RHELSpec{
			DistUpgradeOnBoot:               config.DistUpgradeOnBoot,
			RHELSubscriptionManagerUser:     config.RHELSubscriptionManagerUser,
			RHELSubscriptionManagerPassword: config.RHELSubscriptionManagerPassword,
			RHSMOfflineToken:                config.RHSMOfflineToken,
		}
	}

	return operatingSystemSpec, nil
}

// GetAPIV2NodeCloudSpec returns the api compatible NodeCloudSpec for the given machine
func GetAPIV2NodeCloudSpec(machineSpec clusterv1alpha1.MachineSpec) (*apiv1.NodeCloudSpec, error) {
	decodedProviderSpec, err := providerconfig.GetConfig(machineSpec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine providerConfig: %v", err)
	}

	cloudSpec := &apiv1.NodeCloudSpec{}

	stringProviderConfigValue := func(c *providerconfig.ConfigVarString) string {
		if c == nil {
			return ""
		}
		return c.Value
	}

	boolProviderConfigValue := func(c *providerconfig.ConfigVarBool) bool {
		if c == nil {
			return false
		}
		return c.Value
	}

	switch decodedProviderSpec.CloudProvider {
	case providerconfig.CloudProviderAWS:
		config := &aws.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse aws config: %v", err)
		}

		spotInstanceMaxPrice, spotInstanceInterruptionBehavior, spotInstancePersistentRequest := extractSpotInstanceConfigs(config)

		cloudSpec.AWS = &apiv1.AWSNodeSpec{
			Tags:                             config.Tags,
			VolumeSize:                       config.DiskSize,
			VolumeType:                       stringProviderConfigValue(config.DiskType),
			InstanceType:                     stringProviderConfigValue(config.InstanceType),
			AMI:                              stringProviderConfigValue(config.AMI),
			AvailabilityZone:                 stringProviderConfigValue(config.AvailabilityZone),
			SubnetID:                         stringProviderConfigValue(config.SubnetID),
			AssignPublicIP:                   config.AssignPublicIP,
			IsSpotInstance:                   config.IsSpotInstance,
			SpotInstanceMaxPrice:             spotInstanceMaxPrice,
			SpotInstancePersistentRequest:    spotInstancePersistentRequest,
			SpotInstanceInterruptionBehavior: spotInstanceInterruptionBehavior,
		}
	case providerconfig.CloudProviderAzure:
		config := &azure.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse Azure config: %v", err)
		}
		cloudSpec.Azure = &apiv1.AzureNodeSpec{
			Size:           stringProviderConfigValue(config.VMSize),
			AssignPublicIP: boolProviderConfigValue(config.AssignPublicIP),
			Tags:           config.Tags,
			ImageID:        stringProviderConfigValue(config.ImageID),
			Zones:          config.Zones,
			DataDiskSize:   config.DataDiskSize,
			OSDiskSize:     config.OSDiskSize,
		}
	case providerconfig.CloudProviderDigitalocean:
		config := &digitalocean.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse digitalocean config: %v", err)
		}
		cloudSpec.Digitalocean = &apiv1.DigitaloceanNodeSpec{
			IPv6:       boolProviderConfigValue(config.IPv6),
			Size:       stringProviderConfigValue(config.Size),
			Backups:    boolProviderConfigValue(config.Backups),
			Monitoring: boolProviderConfigValue(config.Monitoring),
		}
		for _, v := range config.Tags {
			cloudSpec.Digitalocean.Tags = append(cloudSpec.Digitalocean.Tags, v.Value)
		}
	case providerconfig.CloudProviderOpenstack:
		config := &openstack.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse openstack config: %v", err)
		}
		cloudSpec.Openstack = &apiv1.OpenstackNodeSpec{
			Flavor:                    stringProviderConfigValue(config.Flavor),
			Image:                     stringProviderConfigValue(config.Image),
			Tags:                      config.Tags,
			AvailabilityZone:          stringProviderConfigValue(config.AvailabilityZone),
			InstanceReadyCheckPeriod:  stringProviderConfigValue(config.InstanceReadyCheckPeriod),
			InstanceReadyCheckTimeout: stringProviderConfigValue(config.InstanceReadyCheckTimeout),
		}
		cloudSpec.Openstack.UseFloatingIP = config.FloatingIPPool != nil && config.FloatingIPPool.Value != ""
		if config.RootDiskSizeGB != nil && *config.RootDiskSizeGB > 0 {
			cloudSpec.Openstack.RootDiskSizeGB = config.RootDiskSizeGB
		}
	case providerconfig.CloudProviderHetzner:
		config := &hetzner.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse hetzner config: %v", err)
		}
		cloudSpec.Hetzner = &apiv1.HetznerNodeSpec{
			Type: stringProviderConfigValue(config.ServerType),
		}
		// MachineDeployments created by KKP will only ever have a single network
		// set, but users might have created ones with many; we have to make a choice
		// here as to what to display in KKP.
		if len(config.Networks) > 0 {
			cloudSpec.Hetzner.Network = config.Networks[0].Value
		}
	case providerconfig.CloudProviderVsphere:
		config := &vsphere.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse vsphere config: %v", err)
		}
		cloudSpec.VSphere = &apiv1.VSphereNodeSpec{
			CPUs:       int(config.CPUs),
			Memory:     int(config.MemoryMB),
			DiskSizeGB: config.DiskSizeGB,
			Template:   config.TemplateVMName.Value,
		}
	case providerconfig.CloudProviderPacket:
		config := &packet.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse packet config: %v", err)
		}
		cloudSpec.Packet = &apiv1.PacketNodeSpec{
			InstanceType: config.InstanceType.Value,
		}
		for _, v := range config.Tags {
			cloudSpec.Packet.Tags = append(cloudSpec.Packet.Tags, v.Value)
		}
	case providerconfig.CloudProviderGoogle:
		config := &gce.CloudProviderSpec{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse gcp config: %v", err)
		}
		cloudSpec.GCP = &apiv1.GCPNodeSpec{
			Zone:        stringProviderConfigValue(config.Zone),
			MachineType: stringProviderConfigValue(config.MachineType),
			DiskSize:    config.DiskSize,
			DiskType:    stringProviderConfigValue(config.DiskType),
			Preemptible: boolProviderConfigValue(config.Preemptible),
			Labels:      config.Labels,
			Tags:        config.Tags,
			CustomImage: stringProviderConfigValue(config.CustomImage),
		}
	case providerconfig.CloudProviderKubeVirt:
		config := &kubevirt.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse kubevirt config: %v", err)
		}

		cloudSpec.Kubevirt = &apiv1.KubevirtNodeSpec{
			CPUs:             stringProviderConfigValue(config.CPUs),
			Memory:           stringProviderConfigValue(config.Memory),
			Namespace:        stringProviderConfigValue(config.Namespace),
			SourceURL:        stringProviderConfigValue(config.SourceURL),
			StorageClassName: stringProviderConfigValue(config.StorageClassName),
			PVCSize:          stringProviderConfigValue(config.PVCSize),
		}
	case providerconfig.CloudProviderAlibaba:
		config := &alibaba.RawConfig{}
		if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
			return nil, fmt.Errorf("failed to parse alibaba config: %v", err)
		}
		cloudSpec.Alibaba = &apiv1.AlibabaNodeSpec{
			InstanceType:            stringProviderConfigValue(config.InstanceType),
			DiskSize:                stringProviderConfigValue(config.DiskSize),
			DiskType:                stringProviderConfigValue(config.DiskType),
			VSwitchID:               stringProviderConfigValue(config.VSwitchID),
			InternetMaxBandwidthOut: stringProviderConfigValue(config.InternetMaxBandwidthOut),
			Labels:                  config.Labels,
			ZoneID:                  stringProviderConfigValue(config.ZoneID),
		}
	case providerconfig.CloudProviderAnexia:
		{
			config := &anexia.RawConfig{}
			if err := json.Unmarshal(decodedProviderSpec.CloudProviderSpec.Raw, &config); err != nil {
				return nil, fmt.Errorf("failed to parse anexia config: %v", err)
			}
			cloudSpec.Anexia = &apiv1.AnexiaNodeSpec{
				VlanID:     stringProviderConfigValue(config.VlanID),
				TemplateID: stringProviderConfigValue(config.TemplateID),
				CPUs:       config.CPUs,
				Memory:     int64(config.Memory),
				DiskSize:   int64(config.DiskSize),
			}
		}
	default:
		return nil, fmt.Errorf("unknown cloud provider %q", decodedProviderSpec.CloudProvider)
	}

	return cloudSpec, nil
}

func extractSpotInstanceConfigs(config *aws.RawConfig) (*string, *string, *bool) {
	if config.IsSpotInstance != nil &&
		*config.IsSpotInstance &&
		config.SpotInstanceConfig != nil {
		var price, behavior string
		var persistentrqst bool
		if config.SpotInstanceConfig.MaxPrice != nil {
			price = config.SpotInstanceConfig.MaxPrice.Value
		}
		if config.SpotInstanceConfig.InterruptionBehavior != nil {
			behavior = config.SpotInstanceConfig.InterruptionBehavior.Value
		}
		if config.SpotInstanceConfig.PersistentRequest != nil {
			persistentrqst = config.SpotInstanceConfig.PersistentRequest.Value
		}
		return &price, &behavior, &persistentrqst
	}

	return nil, nil, nil
}
