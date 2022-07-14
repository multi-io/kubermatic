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

package v2

import (
	apiv1 "k8c.io/kubermatic/v2/pkg/api/v1"
	crdapiv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"

	corev1 "k8s.io/api/core/v1"
)

// GVK group version kind of a resource
type GVK struct {
	Group   string `json:"group,omitempty"`
	Version string `json:"version,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

// PresetList represents a list of presets
// swagger:model PresetList
type PresetList struct {
	Items []Preset `json:"items"`
}

// Preset represents a preset
// swagger:model Preset
type Preset struct {
	Name      string           `json:"name"`
	Enabled   bool             `json:"enabled"`
	Providers []PresetProvider `json:"providers"`
}

// PresetProvider represents a preset provider
// swagger:model PresetProvider
type PresetProvider struct {
	Name    crdapiv1.ProviderType `json:"name"`
	Enabled bool                  `json:"enabled"`
}

// SeedSettings represents settings for a Seed cluster
// swagger:model SeedSettings
type SeedSettings struct {
	// the Seed level seed dns overwrite
	SeedDNSOverwrite string `json:"seedDNSOverwrite,omitempty"`
}

// ClusterTemplate represents a ClusterTemplate object
// swagger:model ClusterTemplate
type ClusterTemplate struct {
	Name string `json:"name"`
	ID   string `json:"id"`

	ProjectID      string                  `json:"projectID,omitempty"`
	User           string                  `json:"user,omitempty"`
	Scope          string                  `json:"scope"`
	UserSSHKeys    []ClusterTemplateSSHKey `json:"userSshKeys,omitempty"`
	Cluster        *apiv1.Cluster          `json:"cluster,omitempty"`
	NodeDeployment *apiv1.NodeDeployment   `json:"nodeDeployment,omitempty"`
}

// ClusterTemplateSSHKey represents SSH Key object for Cluster Template
// swagger:model ClusterTemplateSSHKey
type ClusterTemplateSSHKey struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// ClusterTemplateList represents a ClusterTemplate list
// swagger:model ClusterTemplateList
type ClusterTemplateList []ClusterTemplate

// ClusterTemplateInstance represents a ClusterTemplateInstance object
// swagger:model ClusterTemplateInstance
type ClusterTemplateInstance struct {
	Name string `json:"name"`

	Spec crdapiv1.ClusterTemplateInstanceSpec `json:"spec"`
}

// EtcdBackupConfig represents an object holding the configuration for etcd backups
// swagger:model EtcdBackupConfig
type EtcdBackupConfig struct {
	apiv1.ObjectMeta

	Spec   EtcdBackupConfigSpec   `json:"spec"`
	Status EtcdBackupConfigStatus `json:"status"`
}

type EtcdBackupConfigStatus struct {
	// CurrentBackups tracks the creation and deletion progress if all backups managed by the EtcdBackupConfig
	CurrentBackups []BackupStatus `json:"lastBackups,omitempty"`
	// Conditions contains conditions of the EtcdBackupConfig
	Conditions []EtcdBackupConfigCondition `json:"conditions,omitempty"`
	// If the controller was configured with a cleanupContainer, CleanupRunning keeps track of the corresponding job
	CleanupRunning bool `json:"cleanupRunning,omitempty"`
}

type BackupStatus struct {
	// ScheduledTime will always be set when the BackupStatus is created, so it'll never be nil
	ScheduledTime      *apiv1.Time                `json:"scheduledTime,omitempty"`
	BackupName         string                     `json:"backupName,omitempty"`
	JobName            string                     `json:"jobName,omitempty"`
	BackupStartTime    *apiv1.Time                `json:"backupStartTime,omitempty"`
	BackupFinishedTime *apiv1.Time                `json:"backupFinishedTime,omitempty"`
	BackupPhase        crdapiv1.BackupStatusPhase `json:"backupPhase,omitempty"`
	BackupMessage      string                     `json:"backupMessage,omitempty"`
	DeleteJobName      string                     `json:"deleteJobName,omitempty"`
	DeleteStartTime    *apiv1.Time                `json:"deleteStartTime,omitempty"`
	DeleteFinishedTime *apiv1.Time                `json:"deleteFinishedTime,omitempty"`
	DeletePhase        crdapiv1.BackupStatusPhase `json:"deletePhase,omitempty"`
	DeleteMessage      string                     `json:"deleteMessage,omitempty"`
}

type EtcdBackupConfigCondition struct {
	// Type of EtcdBackupConfig condition.
	Type crdapiv1.EtcdBackupConfigConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime apiv1.Time `json:"lastHeartbeatTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime apiv1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// EtcdBackupConfigSpec represents an object holding the etcd backup configuration specification
// swagger:model EtcdBackupConfigSpec
type EtcdBackupConfigSpec struct {
	// ClusterID is the id of the cluster which will be backed up
	ClusterID string `json:"clusterId"`
	// Schedule is a cron expression defining when to perform
	// the backup. If not set, the backup is performed exactly
	// once, immediately.
	Schedule string `json:"schedule,omitempty"`
	// Keep is the number of backups to keep around before deleting the oldest one
	// If not set, defaults to DefaultKeptBackupsCount. Only used if Schedule is set.
	Keep *int `json:"keep,omitempty"`
}

// EtcdRestore represents an object holding the configuration for etcd backup restore
// swagger:model EtcdRestore
type EtcdRestore struct {
	Name string `json:"name"`

	Spec   EtcdRestoreSpec   `json:"spec"`
	Status EtcdRestoreStatus `json:"status"`
}

type EtcdRestoreStatus struct {
	Phase       crdapiv1.EtcdRestorePhase `json:"phase"`
	RestoreTime *apiv1.Time               `json:"restoreTime,omitempty"`
}

// EtcdRestoreSpec represents an object holding the etcd backup restore configuration specification
// swagger:model EtcdRestoreSpec
type EtcdRestoreSpec struct {
	// ClusterID is the id of the cluster which will be restored from the backup
	ClusterID string `json:"clusterId"`
	// BackupName is the name of the backup to restore from
	BackupName string `json:"backupName"`
	// BackupDownloadCredentialsSecret is the name of a secret in the cluster-xxx namespace containing
	// credentials needed to download the backup
	BackupDownloadCredentialsSecret string `json:"backupDownloadCredentialsSecret,omitempty"`
}

// OIDCSpec contains OIDC params that can be used to access user cluster.
// swagger:model OIDCSpec
type OIDCSpec struct {
	IssuerURL    string `json:"issuerUrl,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// BackupCredentials contains credentials for etcd backups
// swagger:model BackupCredentials
type BackupCredentials struct {
	S3BackupCredentials S3BackupCredentials `json:"s3,omitempty"`
}

// S3BackupCredentials contains credentials for S3 etcd backups
// swagger:model S3BackupCredentials
type S3BackupCredentials struct {
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// FeatureGates represents an object holding feature gate settings
// swagger:model FeatureGates
type FeatureGates struct {
	KonnectivityService *bool `json:"konnectivityService,omitempty"`
}
