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

package openstack

import (
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"

	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	Name = "csi-cinder-controller"
)

// ClusterRoleCreator returns the func to create/update the ClusterRole for the Cinder CSI controller
func ClusterRoleCreator() reconciling.NamedClusterRoleCreatorGetter {
	return func() (string, reconciling.ClusterRoleCreator) {
		return resources.CSICinderControllerClusterRoleName, func(cr *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
			cr.Labels = resources.BaseAppLabels(Name, nil)

			cr.Rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{
						"persistentvolumes",
					},
					Verbs: []string{"get", "list", "watch", "create", "delete", "patch", "update"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{
						"persistentvolumeclaims",
					},
					Verbs: []string{"get", "list", "watch", "update"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{
						"persistentvolumeclaims/status",
					},
					Verbs: []string{"patch"},
				},
				{
					APIGroups: []string{"storage.k8s.io"},
					Resources: []string{
						"storageclasses",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{
						"nodes",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"storage.k8s.io"},
					Resources: []string{
						"volumeattachments",
					},
					Verbs: []string{"get", "list", "watch", "patch", "update"},
				},
				{
					APIGroups: []string{"storage.k8s.io"},
					Resources: []string{
						"csinodes",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{
						"events",
					},
					Verbs: []string{"list", "watch", "create", "update", "patch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{
						"secrets",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{
						"pods",
					},
					Verbs: []string{"get", "list", "watch"},
				},

				{
					APIGroups: []string{"snapshot.storage.k8s.io"},
					Resources: []string{
						"volumesnapshotclasses",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"snapshot.storage.k8s.io"},
					Resources: []string{
						"volumesnapshots",
					},
					Verbs: []string{"get", "list"},
				},
				{
					APIGroups: []string{"snapshot.storage.k8s.io"},
					Resources: []string{
						"volumesnapshotcontents",
					},
					Verbs: []string{"create", "get", "list", "watch", "update", "delete"},
				},
				{
					APIGroups: []string{"snapshot.storage.k8s.io"},
					Resources: []string{
						"volumesnapshotcontents/status",
					},
					Verbs: []string{"update"},
				},
				{
					APIGroups: []string{"storage.k8s.io"},
					Resources: []string{
						"volumeattachments/status",
					},
					Verbs: []string{"patch", "update"},
				},
				{
					APIGroups: []string{"coordination.k8s.io"},
					Resources: []string{
						"leases",
					},
					Verbs: []string{"get", "watch", "list", "delete", "update", "create"},
				},
			}
			return cr, nil
		}
	}
}
