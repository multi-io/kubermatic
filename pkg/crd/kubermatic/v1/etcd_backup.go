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
	// EtcdBackupResourceName represents "Resource" defined in Kubernetes
	EtcdBackupResourceName = "etcdbackup"

	// EtcdBackupKindName represents "Kind" defined in Kubernetes
	EtcdBackupKindName = "EtcdBackup"

	DefaultKeptBackupsCount = 20
	MaxKeptBackupsCount     = 20
)

//+genclient

// EtcdBackup specifies a add-on
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EtcdBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdBackupSpec   `json:"spec"`
	Status EtcdBackupStatus `json:"status,omitempty"`
}

// EtcdBackupSpec specifies details of an etcd backup
type EtcdBackupSpec struct {
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

// EtcdBackupList is a list of etcd backups
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EtcdBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EtcdBackup `json:"items"`
}

type EtcdBackupStatus struct {
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	CurrentBackups []string     `json:"lastBackups,omitempty"`
}

func (b *EtcdBackup) GetKeptBackupsCount() int {
	if b.Spec.Keep == nil {
		return DefaultKeptBackupsCount
	}
	if *b.Spec.Keep <= 0 {
		return 1
	}
	if *b.Spec.Keep > MaxKeptBackupsCount {
		return MaxKeptBackupsCount
	}
	return *b.Spec.Keep
}
