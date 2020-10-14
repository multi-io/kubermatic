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
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"

	corev1 "k8s.io/api/core/v1"
)

type etcdBackupConfigCreatorData interface {
	Cluster() *kubermaticv1.Cluster
}

// BackupConfigCreator returns the function to reconcile the EtcdBackupConfigs.
func BackupConfigCreator(data etcdBackupConfigCreatorData) reconciling.NamedEtcdBackupConfigCreatorGetter {
	return func() (string, reconciling.EtcdBackupConfigCreator) {
		return resources.EtcdDefaultBackupConfigName, func(config *kubermaticv1.EtcdBackupConfig) (*kubermaticv1.EtcdBackupConfig, error) {
			config.Spec.Name = resources.EtcdDefaultBackupConfigName
			config.Spec.Schedule = "*/20 * * * *"
			keep := 20
			config.Spec.Keep = &keep
			config.Spec.Cluster = corev1.ObjectReference{
				Kind:       kubermaticv1.ClusterKindName,
				Name:       data.Cluster().Name,
				UID:        data.Cluster().UID,
				APIVersion: "kubermatic.k8s.io/v1",
			}

			return config, nil
		}
	}
}
