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
	"errors"
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
	vsphere "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vsphere/types"
	providerconfig "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"
	"github.com/kubermatic/machine-controller/pkg/userdata/rhel"
	"github.com/kubermatic/machine-controller/pkg/userdata/sles"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"
	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"

	"github.com/aws/aws-sdk-go/service/ec2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
)

func getOsName(nodeSpec apiv1.NodeSpec) (providerconfig.OperatingSystem, error) {
	if nodeSpec.OperatingSystem.CentOS != nil {
		return providerconfig.OperatingSystemCentOS, nil
	}
	if nodeSpec.OperatingSystem.Ubuntu != nil {
		return providerconfig.OperatingSystemUbuntu, nil
	}
	if nodeSpec.OperatingSystem.SLES != nil {
		return providerconfig.OperatingSystemSLES, nil
	}
	if nodeSpec.OperatingSystem.RHEL != nil {
		return providerconfig.OperatingSystemRHEL, nil
	}
	if nodeSpec.OperatingSystem.Flatcar != nil {
		return providerconfig.OperatingSystemFlatcar, nil
	}

	return "", errors.New("unknown operating system")
}

func stringProviderConfig(s *string) *providerconfig.ConfigVarString {
	if s == nil || *s == "" {
		return nil
	}
	return &providerconfig.ConfigVarString{Value: *s}
}

func boolProviderConfig(v *bool) *providerconfig.ConfigVarBool {
	if v == nil || !*v {
		return nil
	}
	return &providerconfig.ConfigVarBool{Value: *v}
}

func getAWSProviderSpec(c *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	osName, err := getOsName(nodeSpec)
	if err != nil {
		return nil, err
	}
	ami := dc.Spec.AWS.Images[osName]
	if nodeSpec.Cloud.AWS.AMI != "" {
		ami = nodeSpec.Cloud.AWS.AMI
	}

	spotConfig := &aws.SpotInstanceConfig{}
	if nodeSpec.Cloud.AWS.IsSpotInstance != nil && *nodeSpec.Cloud.AWS.IsSpotInstance {
		spotConfig.MaxPrice = stringProviderConfig(nodeSpec.Cloud.AWS.SpotInstanceMaxPrice)
		spotConfig.PersistentRequest = boolProviderConfig(nodeSpec.Cloud.AWS.SpotInstancePersistentRequest)
		spotConfig.InterruptionBehavior = stringProviderConfig(nodeSpec.Cloud.AWS.SpotInstanceInterruptionBehavior)
	}

	config := aws.RawConfig{
		// If the node spec doesn't provide a subnet ID, AWS will just pick the AZ's default subnet.
		SubnetID:           stringProviderConfig(&nodeSpec.Cloud.AWS.SubnetID),
		VpcID:              stringProviderConfig(&c.Spec.Cloud.AWS.VPCID),
		SecurityGroupIDs:   []providerconfig.ConfigVarString{{Value: c.Spec.Cloud.AWS.SecurityGroupID}},
		Region:             stringProviderConfig(&dc.Spec.AWS.Region),
		AvailabilityZone:   stringProviderConfig(&nodeSpec.Cloud.AWS.AvailabilityZone),
		InstanceProfile:    stringProviderConfig(&c.Spec.Cloud.AWS.InstanceProfileName),
		InstanceType:       stringProviderConfig(&nodeSpec.Cloud.AWS.InstanceType),
		DiskType:           stringProviderConfig(&nodeSpec.Cloud.AWS.VolumeType),
		DiskSize:           nodeSpec.Cloud.AWS.VolumeSize,
		AMI:                stringProviderConfig(&ami),
		AssignPublicIP:     nodeSpec.Cloud.AWS.AssignPublicIP,
		IsSpotInstance:     nodeSpec.Cloud.AWS.IsSpotInstance,
		SpotInstanceConfig: spotConfig,
	}
	if config.DiskType.Value == "" {
		config.DiskType.Value = ec2.VolumeTypeGp2
	}
	if config.DiskSize == 0 {
		config.DiskSize = 25
	}

	config.Tags = map[string]string{}
	for key, value := range nodeSpec.Cloud.AWS.Tags {
		config.Tags[key] = value
	}
	config.Tags["kubernetes.io/cluster/"+c.Name] = ""
	config.Tags["system/cluster"] = c.Name
	projectID, ok := c.Labels[kubermaticv1.ProjectIDLabelKey]
	if ok {
		config.Tags["system/project"] = projectID
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getAzureProviderSpec(c *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := azure.RawConfig{
		Location:              stringProviderConfig(&dc.Spec.Azure.Location),
		ResourceGroup:         stringProviderConfig(&c.Spec.Cloud.Azure.ResourceGroup),
		VNetResourceGroup:     stringProviderConfig(&c.Spec.Cloud.Azure.VNetResourceGroup),
		VMSize:                stringProviderConfig(&nodeSpec.Cloud.Azure.Size),
		OSDiskSize:            nodeSpec.Cloud.Azure.OSDiskSize,
		DataDiskSize:          nodeSpec.Cloud.Azure.DataDiskSize,
		VNetName:              stringProviderConfig(&c.Spec.Cloud.Azure.VNetName),
		SubnetName:            stringProviderConfig(&c.Spec.Cloud.Azure.SubnetName),
		RouteTableName:        stringProviderConfig(&c.Spec.Cloud.Azure.RouteTableName),
		AvailabilitySet:       stringProviderConfig(&c.Spec.Cloud.Azure.AvailabilitySet),
		AssignAvailabilitySet: c.Spec.Cloud.Azure.AssignAvailabilitySet,
		SecurityGroupName:     stringProviderConfig(&c.Spec.Cloud.Azure.SecurityGroup),
		Zones:                 nodeSpec.Cloud.Azure.Zones,
		ImageID:               stringProviderConfig(&nodeSpec.Cloud.Azure.ImageID),

		// https://github.com/kubermatic/kubermatic/issues/5013#issuecomment-580357280
		AssignPublicIP: boolProviderConfig(&nodeSpec.Cloud.Azure.AssignPublicIP),
	}
	config.Tags = map[string]string{}
	for key, value := range nodeSpec.Cloud.Azure.Tags {
		config.Tags[key] = value
	}
	config.Tags["KubernetesCluster"] = c.Name
	config.Tags["system-cluster"] = c.Name
	projectID, ok := c.Labels[kubermaticv1.ProjectIDLabelKey]
	if ok {
		config.Tags["system-project"] = projectID
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getVSphereProviderSpec(c *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	var datastore = ""
	// If `DatastoreCluster` is not specified we use either the Datastore
	// specified at `Cluster` or the one specified at `Datacenter` level.
	if c.Spec.Cloud.VSphere.DatastoreCluster == "" {
		datastore = defaultIfEmpty(c.Spec.Cloud.VSphere.Datastore, dc.Spec.VSphere.DefaultDatastore)
	}

	config := vsphere.RawConfig{
		TemplateVMName:   stringProviderConfig(&nodeSpec.Cloud.VSphere.Template),
		VMNetName:        stringProviderConfig(&c.Spec.Cloud.VSphere.VMNetName),
		CPUs:             int32(nodeSpec.Cloud.VSphere.CPUs),
		MemoryMB:         int64(nodeSpec.Cloud.VSphere.Memory),
		DiskSizeGB:       nodeSpec.Cloud.VSphere.DiskSizeGB,
		Datacenter:       stringProviderConfig(&dc.Spec.VSphere.Datacenter),
		Datastore:        stringProviderConfig(&datastore),
		DatastoreCluster: stringProviderConfig(&c.Spec.Cloud.VSphere.DatastoreCluster),
		Cluster:          stringProviderConfig(&dc.Spec.VSphere.Cluster),
		Folder:           stringProviderConfig(&c.Spec.Cloud.VSphere.Folder),
		AllowInsecure:    boolProviderConfig(&dc.Spec.VSphere.AllowInsecure),
		ResourcePool:     stringProviderConfig(&c.Spec.Cloud.VSphere.ResourcePool),
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

// EnsureNoUnnecessaryOpenstackProviderSpecDiff deletes empty fields that might cause node rollover. Returns new machine deployment.
func EnsureNoUnnecessaryOpenstackProviderSpecDiff(mdBefore, mdNow *clusterv1alpha1.MachineDeployment) (*clusterv1alpha1.MachineDeployment, error) {
	if mdBefore.Spec.Template.Spec.ProviderSpec.Value == nil || mdNow.Spec.Template.Spec.ProviderSpec.Value == nil {
		return mdNow, nil
	}
	before, err := extractOpenstackConfig(mdBefore.Spec.Template)
	if err != nil {
		return nil, err
	}
	after, err := extractOpenstackConfig(mdNow.Spec.Template)
	if err != nil {
		return nil, err
	}

	preserved := preserveBlankFieldsInConfig(before, after)
	// Note(fkasymovgeniiuulu): We want to delete this code. It ensures old machine deployments with empty fields don't
	// get node rollovers. We can delete this code in the future when none of machine deployments have empty fields.
	// To come to this state sooner, we do not preserve empty fields in machine deployments if node rollover will happen
	// anyway due change of flavor or image etc.
	rollover := openstackConfigNotEqual(before, preserved)
	if rollover {
		return mdNow, nil
	}

	result := mdNow.DeepCopy()
	result.Spec.Template.Spec.ProviderSpec, err = writeOpenstackProviderConfig(result.Spec.Template.Spec.ProviderSpec, preserved)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func openstackConfigNotEqual(a, b *openstack.RawConfig) bool {
	snot := func(x, y *providerconfig.ConfigVarString) bool {
		return x != y && (x == nil || y == nil || x.Value != y.Value ||
			x.ConfigMapKeyRef != y.ConfigMapKeyRef || x.SecretKeyRef != y.SecretKeyRef)
	}
	bnot := func(x, y *providerconfig.ConfigVarBool) bool {
		return x != y && (x == nil || y == nil || x.Value != y.Value ||
			x.ConfigMapKeyRef != y.ConfigMapKeyRef || x.SecretKeyRef != y.SecretKeyRef)
	}
	return snot(a.IdentityEndpoint, b.IdentityEndpoint) ||
		snot(a.Username, b.Username) ||
		snot(a.Password, b.Password) ||
		snot(a.ApplicationCredentialID, b.ApplicationCredentialID) ||
		snot(a.ApplicationCredentialSecret, b.ApplicationCredentialSecret) ||
		snot(a.DomainName, b.DomainName) ||
		snot(a.ProjectName, b.ProjectName) ||
		snot(a.ProjectID, b.ProjectID) ||
		snot(a.TenantName, b.TenantName) ||
		snot(a.TenantID, b.TenantID) ||
		snot(a.TokenID, b.TokenID) ||
		snot(a.Region, b.Region) ||
		snot(a.InstanceReadyCheckPeriod, b.InstanceReadyCheckPeriod) ||
		snot(a.InstanceReadyCheckTimeout, b.InstanceReadyCheckTimeout) ||
		snot(a.ComputeAPIVersion, b.ComputeAPIVersion) ||
		snot(a.Image, b.Image) ||
		snot(a.Flavor, b.Flavor) ||
		snot(a.Network, b.Network) ||
		snot(a.Subnet, b.Subnet) ||
		snot(a.FloatingIPPool, b.FloatingIPPool) ||
		snot(a.AvailabilityZone, b.AvailabilityZone) ||
		bnot(a.TrustDevicePath, b.TrustDevicePath) ||
		(a.RootDiskSizeGB != nil && b.RootDiskSizeGB != nil && *a.RootDiskSizeGB != *b.RootDiskSizeGB) ||
		snot(a.RootDiskVolumeType, b.RootDiskVolumeType) ||
		(a.NodeVolumeAttachLimit != nil && b.NodeVolumeAttachLimit != nil && *a.NodeVolumeAttachLimit != *b.NodeVolumeAttachLimit) ||
		snot(a.ServerGroupID, b.ServerGroupID)
}

func extractOpenstackConfig(template clusterv1alpha1.MachineTemplateSpec) (*openstack.RawConfig, error) {
	pc, err := extractProviderConfig(template.Spec.ProviderSpec)
	if err != nil {
		return nil, err
	}
	result := new(openstack.RawConfig)
	err = json.Unmarshal(pc.CloudProviderSpec.Raw, result)
	return result, err
}

func extractProviderConfig(spec clusterv1alpha1.ProviderSpec) (*providerconfig.Config, error) {
	result := &providerconfig.Config{}
	err := json.Unmarshal(spec.Value.Raw, result)
	return result, err
}

func writeOpenstackProviderConfig(spec clusterv1alpha1.ProviderSpec, c *openstack.RawConfig) (clusterv1alpha1.ProviderSpec, error) {
	config := &providerconfig.Config{}
	err := json.Unmarshal(spec.Value.Raw, config)
	if err != nil {
		return spec, err
	}
	config.CloudProviderSpec.Raw, err = json.Marshal(c)
	if err != nil {
		return spec, err
	}
	spec.Value.Raw, err = json.Marshal(config)
	if err != nil {
		return spec, err
	}
	return spec, nil
}

func getOpenstackProviderSpec(c *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := &openstack.RawConfig{
		Image:                 stringProviderConfig(&nodeSpec.Cloud.Openstack.Image),
		Flavor:                stringProviderConfig(&nodeSpec.Cloud.Openstack.Flavor),
		AvailabilityZone:      stringProviderConfig(&dc.Spec.Openstack.AvailabilityZone),
		Region:                stringProviderConfig(&dc.Spec.Openstack.Region),
		IdentityEndpoint:      stringProviderConfig(&dc.Spec.Openstack.AuthURL),
		Network:               stringProviderConfig(&c.Spec.Cloud.Openstack.Network),
		Subnet:                stringProviderConfig(&c.Spec.Cloud.Openstack.SubnetID),
		NodeVolumeAttachLimit: dc.Spec.Openstack.NodeVolumeAttachLimit,
		// Sys11: Set soft-anti-affinity server group name to be set on machines as schedule hint
		ServerGroupID:             stringProviderConfig(&c.Spec.Cloud.Openstack.ServerGroupID),
		InstanceReadyCheckPeriod:  stringProviderConfig(&nodeSpec.Cloud.Openstack.InstanceReadyCheckPeriod),
		InstanceReadyCheckTimeout: stringProviderConfig(&nodeSpec.Cloud.Openstack.InstanceReadyCheckTimeout),
	}

	if v := c.Spec.Cloud.Openstack.SecurityGroups; v != "" {
		config.SecurityGroups = []providerconfig.ConfigVarString{{Value: c.Spec.Cloud.Openstack.SecurityGroups}}
	}

	if nodeSpec.Cloud.Openstack.UseFloatingIP || dc.Spec.Openstack.EnforceFloatingIP {
		config.FloatingIPPool = stringProviderConfig(&c.Spec.Cloud.Openstack.FloatingIPPool)
	}

	if nodeSpec.Cloud.Openstack.RootDiskSizeGB != nil && *nodeSpec.Cloud.Openstack.RootDiskSizeGB > 0 {
		config.RootDiskSizeGB = nodeSpec.Cloud.Openstack.RootDiskSizeGB
	}

	if dc.Spec.Openstack.TrustDevicePath != nil {
		config.TrustDevicePath = boolProviderConfig(dc.Spec.Openstack.TrustDevicePath)
	}

	// Use the nodeDeployment spec AvailabilityZone if set, otherwise we stick to the default from the datacenter
	if nodeSpec.Cloud.Openstack.AvailabilityZone != "" {
		config.AvailabilityZone = stringProviderConfig(&nodeSpec.Cloud.Openstack.AvailabilityZone)
	}

	config.Tags = map[string]string{}
	for key, value := range nodeSpec.Cloud.Openstack.Tags {
		config.Tags[key] = value
	}
	config.Tags["metakube-cluster"] = c.Name
	config.Tags["system-cluster"] = c.Name
	projectID, ok := c.Labels[kubermaticv1.ProjectIDLabelKey]
	if ok {
		config.Tags["system-project"] = projectID
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getHetznerProviderSpec(_ *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := hetzner.RawConfig{
		Datacenter: &providerconfig.ConfigVarString{Value: dc.Spec.Hetzner.Datacenter},
		Location:   &providerconfig.ConfigVarString{Value: dc.Spec.Hetzner.Location},
		Networks:   []providerconfig.ConfigVarString{{Value: dc.Spec.Hetzner.Network}},
		ServerType: &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Hetzner.Type},
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getDigitaloceanProviderSpec(c *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := digitalocean.RawConfig{
		Region:            &providerconfig.ConfigVarString{Value: dc.Spec.Digitalocean.Region},
		Backups:           &providerconfig.ConfigVarBool{Value: nodeSpec.Cloud.Digitalocean.Backups},
		IPv6:              &providerconfig.ConfigVarBool{Value: nodeSpec.Cloud.Digitalocean.IPv6},
		Monitoring:        &providerconfig.ConfigVarBool{Value: nodeSpec.Cloud.Digitalocean.Monitoring},
		Size:              &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Digitalocean.Size},
		PrivateNetworking: &providerconfig.ConfigVarBool{Value: true},
	}

	tags := sets.NewString(nodeSpec.Cloud.Digitalocean.Tags...)
	tags.Insert("metakube", fmt.Sprintf("metakube-cluster-%s", c.Name), fmt.Sprintf("system-cluster-%s", c.Name))
	projectID, ok := c.Labels[kubermaticv1.ProjectIDLabelKey]
	if ok {
		tags.Insert(fmt.Sprintf("system-project-%s", projectID))
	}

	config.Tags = make([]providerconfig.ConfigVarString, len(tags.List()))
	for i, tag := range tags.List() {
		config.Tags[i].Value = tag
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getGCPProviderSpec(c *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, _ *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := gce.CloudProviderSpec{
		Zone:                  &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.GCP.Zone},
		MachineType:           &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.GCP.MachineType},
		DiskSize:              nodeSpec.Cloud.GCP.DiskSize,
		DiskType:              &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.GCP.DiskType},
		Preemptible:           &providerconfig.ConfigVarBool{Value: nodeSpec.Cloud.GCP.Preemptible},
		Network:               &providerconfig.ConfigVarString{Value: c.Spec.Cloud.GCP.Network},
		Subnetwork:            &providerconfig.ConfigVarString{Value: c.Spec.Cloud.GCP.Subnetwork},
		AssignPublicIPAddress: &providerconfig.ConfigVarBool{Value: true},
		CustomImage:           &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.GCP.CustomImage},
	}

	tags := sets.NewString(nodeSpec.Cloud.GCP.Tags...)
	tags.Insert(fmt.Sprintf("kubernetes-cluster-%s", c.Name), fmt.Sprintf("system-cluster-%s", c.Name))
	projectID, ok := c.Labels[kubermaticv1.ProjectIDLabelKey]
	if ok {
		tags.Insert(fmt.Sprintf("system-project-%s", projectID))
	}
	config.Tags = tags.List()

	config.Labels = map[string]string{}
	for key, value := range nodeSpec.Cloud.GCP.Labels {
		config.Labels[key] = value
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getKubevirtProviderSpec(nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := kubevirt.RawConfig{
		CPUs:             &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Kubevirt.CPUs},
		PVCSize:          &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Kubevirt.PVCSize},
		StorageClassName: &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Kubevirt.StorageClassName},
		SourceURL:        &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Kubevirt.SourceURL},
		Namespace:        &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Kubevirt.Namespace},
		Memory:           &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Kubevirt.Memory},
		DNSPolicy:        &providerconfig.ConfigVarString{Value: dc.Spec.Kubevirt.DNSPolicy},
		DNSConfig:        dc.Spec.Kubevirt.DNSConfig,
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getAlibabaProviderSpec(_ *kubermaticv1.Cluster, nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := alibaba.RawConfig{
		InstanceType:            &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Alibaba.InstanceType},
		DiskSize:                &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Alibaba.DiskSize},
		DiskType:                &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Alibaba.DiskType},
		VSwitchID:               &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Alibaba.VSwitchID},
		RegionID:                &providerconfig.ConfigVarString{Value: dc.Spec.Alibaba.Region},
		InternetMaxBandwidthOut: &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Alibaba.InternetMaxBandwidthOut},
		ZoneID:                  &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Alibaba.ZoneID},
	}

	config.Labels = map[string]string{}
	for key, value := range nodeSpec.Cloud.Alibaba.Labels {
		config.Labels[key] = value
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getAnexiaProviderSpec(nodeSpec apiv1.NodeSpec, dc *kubermaticv1.Datacenter) (*runtime.RawExtension, error) {
	config := anexia.RawConfig{
		VlanID:     &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Anexia.VlanID},
		TemplateID: &providerconfig.ConfigVarString{Value: nodeSpec.Cloud.Anexia.TemplateID},
		CPUs:       nodeSpec.Cloud.Anexia.CPUs,
		Memory:     int(nodeSpec.Cloud.Anexia.Memory),
		DiskSize:   int(nodeSpec.Cloud.Anexia.DiskSize),
		LocationID: &providerconfig.ConfigVarString{Value: dc.Spec.Anexia.LocationID},
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getCentOSOperatingSystemSpec(nodeSpec apiv1.NodeSpec) (*runtime.RawExtension, error) {
	config := centos.Config{
		DistUpgradeOnBoot: nodeSpec.OperatingSystem.CentOS.DistUpgradeOnBoot,
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getUbuntuOperatingSystemSpec(nodeSpec apiv1.NodeSpec) (*runtime.RawExtension, error) {
	config := ubuntu.Config{
		DistUpgradeOnBoot: nodeSpec.OperatingSystem.Ubuntu.DistUpgradeOnBoot,
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getSLESOperatingSystemSpec(nodeSpec apiv1.NodeSpec) (*runtime.RawExtension, error) {
	config := sles.Config{
		DistUpgradeOnBoot: nodeSpec.OperatingSystem.SLES.DistUpgradeOnBoot,
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getRHELOperatingSystemSpec(nodeSpec apiv1.NodeSpec) (*runtime.RawExtension, error) {
	config := rhel.Config{
		DistUpgradeOnBoot:               nodeSpec.OperatingSystem.RHEL.DistUpgradeOnBoot,
		RHELSubscriptionManagerUser:     nodeSpec.OperatingSystem.RHEL.RHELSubscriptionManagerUser,
		RHELSubscriptionManagerPassword: nodeSpec.OperatingSystem.RHEL.RHELSubscriptionManagerPassword,
		RHSMOfflineToken:                nodeSpec.OperatingSystem.RHEL.RHSMOfflineToken,
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

func getFlatcarOperatingSystemSpec(nodeSpec apiv1.NodeSpec) (*runtime.RawExtension, error) {
	config := flatcar.Config{
		DisableAutoUpdate: nodeSpec.OperatingSystem.Flatcar.DisableAutoUpdate,
		// We manage Flatcar updates via the CoreOS update operator which requires locksmithd
		// to be disabled: https://github.com/coreos/container-linux-update-operator#design
		DisableLocksmithD: true,

		ProvisioningUtility: flatcar.Ignition,
	}
	// set cloud init only for anexia and aws(due to the userdata size limit on aws, ignition increases the size drastically).
	// This should be temporary until the new operating system manager is added to KKP.
	if nodeSpec.Cloud.Anexia != nil || nodeSpec.Cloud.AWS != nil {
		config.ProvisioningUtility = flatcar.CloudInit
	}

	ext := &runtime.RawExtension{}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}

// defaultIfEmpty returns the given value if not empty or the default value
// otherwise.
func defaultIfEmpty(value, defaultValue string) string {
	if value != "" {
		return value
	}
	return defaultValue
}
