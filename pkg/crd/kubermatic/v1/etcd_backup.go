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

	EtcdBackupCreated EtcdBackupConditionType = "EtcdBackupCreatedSuccessfully"
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
	Name string `json:"name"`
	// Cluster is the reference to the cluster whose etcd will be backed up
	Cluster corev1.ObjectReference `json:"cluster"`
	// TTL is an optional time.Duration-parseable string specifying how long
	// the EtcdBackup should be retained. If not set, the backup will be kept until
	// deleted explicitly
	TTL *metav1.Duration `json:"ttl,omitempty"`
}

// EtcdBackupList is a list of etcd backups
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type EtcdBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EtcdBackup `json:"items"`
}

type EtcdBackupStatus struct {
	Conditions []EtcdBackupCondition `json:"conditions,omitempty"`
}

type EtcdBackupConditionType string

type EtcdBackupCondition struct {
	// Type of addon condition.
	Type EtcdBackupConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// HasConditionValue returns true if the EtcdBackup status has the given condition with the given status.
// It does not verify that the condition has been set by a certain Kubermatic version, it just checks
// the existence.
func (bs *EtcdBackupStatus) HasConditionValue(conditionType EtcdBackupConditionType, conditionStatus corev1.ConditionStatus) bool {
	for _, condition := range bs.Conditions {
		if condition.Type == conditionType {
			return condition.Status == conditionStatus
		}
	}

	return false
}
