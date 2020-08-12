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

package etcd

import (
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"

	corev1 "k8s.io/api/core/v1"
)

type backupS3SettingsSecretCreatorData interface {
	BackupS3Endpoint() string
	BackupS3BucketName() string
	BackupS3AccessKeyID() string
	BackupS3SecretAccessKey() string
}

// BackupS3SettingsSecretCreator returns a function to create/update the secret with the s3 settings needed for accessing etcd backups
func BackupS3SettingsSecretCreator(data backupS3SettingsSecretCreatorData) reconciling.NamedSecretCreatorGetter {
	return func() (string, reconciling.SecretCreator) {
		return resources.EtcdBackupS3SettingsSecretName, func(se *corev1.Secret) (*corev1.Secret, error) {
			if se.Data == nil {
				se.Data = map[string][]byte{}
			}
			se.Data[resources.EtcdBackupS3EndpointSecretKey] = []byte(data.BackupS3Endpoint())
			se.Data[resources.EtcdBackupS3BucketNameSecretKey] = []byte(data.BackupS3BucketName())
			se.Data[resources.EtcdBackupS3AccessKeyIDSecretKey] = []byte(data.BackupS3AccessKeyID())
			se.Data[resources.EtcdBackupS3SecretAccessKeySecretKey] = []byte(data.BackupS3SecretAccessKey())
			return se, nil
		}
	}
}
