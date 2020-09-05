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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// EtcdBackupConfigResourceName represents "Resource" defined in Kubernetes
	EtcdBackupConfigResourceName = "etcdbackupconfig"

	// EtcdBackupConfigKindName represents "Kind" defined in Kubernetes
	EtcdBackupConfigKindName = "EtcdBackupConfig"

	DefaultKeptBackupsCount = 20
	MaxKeptBackupsCount     = 20

	// BackupStatusPhase value indicating that the corresponding job has started
	BackupStatusPhaseRunning = "Running"

	// BackupStatusPhase value indicating that the corresponding job has completed successfully
	BackupStatusPhaseCompleted = "Completed"

	// BackupStatusPhase value indicating that the corresponding job has completed with an error
	BackupStatusPhaseFailed = "Failed"
)

//+genclient

// EtcdBackupConfig specifies a add-on
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EtcdBackupConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdBackupConfigSpec   `json:"spec"`
	Status EtcdBackupConfigStatus `json:"status,omitempty"`
}

// EtcdBackupConfigSpec specifies details of an etcd backup
type EtcdBackupConfigSpec struct {
	// Name defines the name of the backup
	// The name of the backup file in S3 will be <cluster>-<backup name>
	// If a schedule is set (see below), -<timestamp> will be appended.
	Name string `json:"name"`
	// Cluster is the reference to the cluster whose etcd will be backed up
	Cluster corev1.ObjectReference `json:"cluster"`
	// Schedule is a cron expression defining when to perform
	// the backup. If not set, the backup is performed exactly
	// once, immediately.
	Schedule string `json:"schedule,omitempty"`
	// Keep is the number of backups to keep around before deleting the oldest one
	// If not set, defaults to DefaultKeptBackupsCount. Only used if Schedule is set.
	Keep *int `json:"keep,omitempty"`
}

// EtcdBackupConfigList is a list of etcd backup configs
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EtcdBackupConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EtcdBackupConfig `json:"items"`
}

type EtcdBackupConfigStatus struct {
	CurrentBackups []BackupStatus `json:"lastBackups,omitempty"`
}

type BackupStatusPhase string

type BackupStatus struct {
	// ScheduledTime will always be set when the BackupStatus is created, so it'll never be nil
	ScheduledTime      *metav1.Time      `json:"scheduledTime,omitempty"`
	BackupName         string            `json:"backupName,omitempty"`
	JobName            string            `json:"jobName,omitempty"`
	BackupStartTime    *metav1.Time      `json:"backupStartTime,omitempty"`
	BackupFinishedTime *metav1.Time      `json:"backupFinishedTime,omitempty"`
	BackupPhase        BackupStatusPhase `json:"backupPhase,omitempty"`
	BackupMessage      string            `json:"backupMessage,omitempty"`
	DeleteJobName      string            `json:"deleteJobName,omitempty"`
	DeleteStartTime    *metav1.Time      `json:"deleteStartTime,omitempty"`
	DeleteFinishedTime *metav1.Time      `json:"deleteFinishedTime,omitempty"`
	DeletePhase        BackupStatusPhase `json:"deletePhase,omitempty"`
	DeleteMessage      string            `json:"deleteMessage,omitempty"`
}

func (bc *EtcdBackupConfig) GetKeptBackupsCount() int {
	if bc.Spec.Keep == nil {
		return DefaultKeptBackupsCount
	}
	if *bc.Spec.Keep <= 0 {
		return 1
	}
	if *bc.Spec.Keep > MaxKeptBackupsCount {
		return MaxKeptBackupsCount
	}
	return *bc.Spec.Keep
}
