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

package cluster_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiv1 "github.com/kubermatic/kubermatic/api/pkg/api/v1"
	"github.com/kubermatic/kubermatic/api/pkg/handler/test"
	"github.com/kubermatic/kubermatic/api/pkg/handler/test/hack"
	"github.com/kubermatic/kubermatic/api/pkg/handler/v1/cluster"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBindUserToRole(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name                   string
		body                   string
		roleName               string
		namespace              string
		expectedResponse       string
		httpStatus             int
		clusterToGet           string
		existingAPIUser        *apiv1.User
		existingKubermaticObjs []runtime.Object
		existingKubernrtesObjs []runtime.Object
	}{
		// scenario 1
		{
			name:             "scenario 1: bind user to role test-1",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"namespace":"default","subjects":[{"kind":"User","apiGroup":"rbac.authorization.k8s.io","name":"test@example.com"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-1", "test"),
				genDefaultClusterRole("role-1"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 2: create role binding when cluster role doesn't exist",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"error":{"code":404,"message":"roles.rbac.authorization.k8s.io \"role-1\" not found"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusNotFound,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// scenario 3
		{
			name:             "scenario 3: update existing binding for the new user",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"namespace":"default","subjects":[{"kind":"User","name":"bob@acme.com"},{"kind":"User","apiGroup":"rbac.authorization.k8s.io","name":"test@example.com"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRoleBinding("test", "default", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 4: bind existing user",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"error":{"code":400,"message":"user bob@acme.com already connected to role role-1"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusBadRequest,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRoleBinding("test", "default", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// group scenarios
		{
			name:             "scenario 5: bind group to role test-1",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"group":"test"}`,
			expectedResponse: `{"namespace":"default","subjects":[{"kind":"Group","apiGroup":"rbac.authorization.k8s.io","name":"test"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-1", "test"),
				genDefaultClusterRole("role-1"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 6: create role binding when cluster role doesn't exist",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"group":"test"}`,
			expectedResponse: `{"error":{"code":404,"message":"roles.rbac.authorization.k8s.io \"role-1\" not found"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusNotFound,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// scenario 7
		{
			name:             "scenario 7: update existing binding for the new group",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"group":"test"}`,
			expectedResponse: `{"namespace":"default","subjects":[{"kind":"Group","name":"admins"},{"kind":"Group","apiGroup":"rbac.authorization.k8s.io","name":"test"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultGroupRoleBinding("test", "default", "role-1", "admins"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 8: update existing binding for the new group",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"group":"test"}`,
			expectedResponse: `{"namespace":"default","subjects":[{"kind":"User","name":"bob@acme.com"},{"kind":"Group","apiGroup":"rbac.authorization.k8s.io","name":"test"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRoleBinding("test", "default", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 9: bind existing group",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"group":"test"}`,
			expectedResponse: `{"error":{"code":400,"message":"group test already connected to role role-1"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusBadRequest,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultGroupRoleBinding("test", "default", "role-1", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 10: the admin John can bind user to role test-1 for any cluster",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"namespace":"default","subjects":[{"kind":"User","apiGroup":"rbac.authorization.k8s.io","name":"test@example.com"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", true),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-1", "test"),
				genDefaultClusterRole("role-1"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
		{
			name:             "scenario 11: the user John can not bind user to role test-1 for Bob's cluster",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"error":{"code":403,"message":"forbidden: \"john@acme.com\" doesn't belong to the given project = my-first-project-ID"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusForbidden,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", false),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-1", "test"),
				genDefaultClusterRole("role-1"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var kubernetesObj []runtime.Object
			var kubeObj []runtime.Object
			kubeObj = append(kubeObj, tc.existingKubernrtesObjs...)
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/roles/%s/%s/bindings", test.ProjectName, tc.clusterToGet, tc.namespace, tc.roleName), strings.NewReader(tc.body))
			res := httptest.NewRecorder()
			var kubermaticObj []runtime.Object
			kubermaticObj = append(kubermaticObj, tc.existingKubermaticObjs...)
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.existingAPIUser, nil, kubeObj, kubernetesObj, kubermaticObj, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.httpStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.httpStatus, res.Code, res.Body.String())
			}

			test.CompareWithResult(t, res, tc.expectedResponse)
		})
	}
}

func TestUnbindUserFromRoleBinding(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name                   string
		body                   string
		roleName               string
		namespace              string
		expectedResponse       string
		httpStatus             int
		clusterToGet           string
		existingAPIUser        *apiv1.User
		existingKubermaticObjs []runtime.Object
		existingKubernrtesObjs []runtime.Object
	}{
		{
			name:             "scenario 1: remove user from the binding",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"namespace":"default","roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRoleBinding("test", "default", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 2: remove group from the binding",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"group":"test"}`,
			expectedResponse: `{"namespace":"default","roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultGroupRoleBinding("test", "default", "role-1", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 3: the admin John can remove user from the binding for the any cluster",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"namespace":"default","roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", true),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRoleBinding("test", "default", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
		{
			name:             "scenario 4: the user John can not remove user from the binding for the Bob's cluster",
			roleName:         "role-1",
			namespace:        "default",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"error":{"code":403,"message":"forbidden: \"john@acme.com\" doesn't belong to the given project = my-first-project-ID"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusForbidden,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", false),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRoleBinding("test", "default", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var kubernetesObj []runtime.Object
			var kubeObj []runtime.Object
			kubeObj = append(kubeObj, tc.existingKubernrtesObjs...)
			req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/roles/%s/%s/bindings", test.ProjectName, tc.clusterToGet, tc.namespace, tc.roleName), strings.NewReader(tc.body))
			res := httptest.NewRecorder()
			var kubermaticObj []runtime.Object
			kubermaticObj = append(kubermaticObj, tc.existingKubermaticObjs...)
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.existingAPIUser, nil, kubeObj, kubernetesObj, kubermaticObj, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.httpStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.httpStatus, res.Code, res.Body.String())
			}

			test.CompareWithResult(t, res, tc.expectedResponse)
		})
	}
}

func TestListRoleBinding(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name                   string
		expectedResponse       string
		httpStatus             int
		clusterToGet           string
		existingAPIUser        *apiv1.User
		existingKubermaticObjs []runtime.Object
		existingKubernrtesObjs []runtime.Object
	}{
		{
			name:             "scenario 1: list bindings",
			expectedResponse: `[{"namespace":"default","subjects":[{"kind":"User","name":"test-1@example.com"}],"roleRefName":"role-1"},{"namespace":"default","subjects":[{"kind":"User","name":"test-2@example.com"}],"roleRefName":"role-2"},{"namespace":"default","subjects":[{"kind":"Group","name":"test"}],"roleRefName":"role-2"},{"namespace":"test","subjects":[{"kind":"User","name":"test-10@example.com"}],"roleRefName":"role-10"},{"namespace":"test","subjects":[{"kind":"Group","name":"test"}],"roleRefName":"role-10"}]`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-2", "default"),
				genDefaultClusterRole("role-1"),
				genDefaultRoleBinding("binding-1", "default", "role-1", "test-1@example.com"),
				genDefaultRoleBinding("binding-2", "default", "role-2", "test-2@example.com"),
				genDefaultGroupRoleBinding("binding-3", "default", "role-2", "test"),
				genDefaultRoleBinding("binding-1", "test", "role-10", "test-10@example.com"),
				genDefaultGroupRoleBinding("binding-2", "test", "role-10", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 2: the admin John can list bindings for any cluster",
			expectedResponse: `[{"namespace":"default","subjects":[{"kind":"User","name":"test-1@example.com"}],"roleRefName":"role-1"},{"namespace":"default","subjects":[{"kind":"User","name":"test-2@example.com"}],"roleRefName":"role-2"},{"namespace":"default","subjects":[{"kind":"Group","name":"test"}],"roleRefName":"role-2"},{"namespace":"test","subjects":[{"kind":"User","name":"test-10@example.com"}],"roleRefName":"role-10"},{"namespace":"test","subjects":[{"kind":"Group","name":"test"}],"roleRefName":"role-10"}]`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", true),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-2", "default"),
				genDefaultClusterRole("role-1"),
				genDefaultRoleBinding("binding-1", "default", "role-1", "test-1@example.com"),
				genDefaultRoleBinding("binding-2", "default", "role-2", "test-2@example.com"),
				genDefaultGroupRoleBinding("binding-3", "default", "role-2", "test"),
				genDefaultRoleBinding("binding-1", "test", "role-10", "test-10@example.com"),
				genDefaultGroupRoleBinding("binding-2", "test", "role-10", "test"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
		{
			name:             "scenario 3: the user John can not list Bob's bindings",
			expectedResponse: `{"error":{"code":403,"message":"forbidden: \"john@acme.com\" doesn't belong to the given project = my-first-project-ID"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusForbidden,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", false),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultRole("role-1", "default"),
				genDefaultRole("role-2", "default"),
				genDefaultClusterRole("role-1"),
				genDefaultRoleBinding("binding-1", "default", "role-1", "test-1@example.com"),
				genDefaultRoleBinding("binding-2", "default", "role-2", "test-2@example.com"),
				genDefaultGroupRoleBinding("binding-3", "default", "role-2", "test"),
				genDefaultRoleBinding("binding-1", "test", "role-10", "test-10@example.com"),
				genDefaultGroupRoleBinding("binding-2", "test", "role-10", "test"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var kubernetesObj []runtime.Object
			var kubeObj []runtime.Object
			var kubermaticObj []runtime.Object
			kubeObj = append(kubeObj, tc.existingKubernrtesObjs...)
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/bindings", test.ProjectName, tc.clusterToGet), strings.NewReader(""))
			res := httptest.NewRecorder()

			kubermaticObj = append(kubermaticObj, tc.existingKubermaticObjs...)
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.existingAPIUser, nil, kubeObj, kubernetesObj, kubermaticObj, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.httpStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.httpStatus, res.Code, res.Body.String())
			}

			test.CompareWithResult(t, res, tc.expectedResponse)
		})
	}
}

func genDefaultRoleBinding(name, namespace, roleID, userEmail string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Labels:    map[string]string{cluster.UserClusterComponentKey: cluster.UserClusterBindingComponentValue},
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "User",
				Name: userEmail,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: roleID,
		},
	}
}

func genDefaultGroupRoleBinding(name, namespace, roleID, group string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Labels:    map[string]string{cluster.UserClusterComponentKey: cluster.UserClusterBindingComponentValue},
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "Group",
				Name: group,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: roleID,
		},
	}
}

func genDefaultClusterRoleBinding(name, roleID, userEmail string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{cluster.UserClusterComponentKey: cluster.UserClusterBindingComponentValue},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "User",
				Name: userEmail,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: roleID,
		},
	}
}

func genDefaultGroupClusterRoleBinding(name, roleID, group string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{cluster.UserClusterComponentKey: cluster.UserClusterBindingComponentValue},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "Group",
				Name: group,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: roleID,
		},
	}
}

func TestBindUserToClusterRole(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name                   string
		body                   string
		roleName               string
		expectedResponse       string
		httpStatus             int
		clusterToGet           string
		existingAPIUser        *apiv1.User
		existingKubermaticObjs []runtime.Object
		existingKubernrtesObjs []runtime.Object
	}{
		// scenario 1
		{
			name:             "scenario 1: bind user to role-1, when cluster role binding doesn't exist",
			roleName:         "role-1",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"error":{"code":500,"message":"the cluster role binding not found"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusInternalServerError,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 2: create cluster role binding when cluster role doesn't exist",
			roleName:         "role-1",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"error":{"code":404,"message":"clusterroles.rbac.authorization.k8s.io \"role-1\" not found"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusNotFound,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-2"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// scenario 3
		{
			name:             "scenario 3: update existing binding for the new user",
			roleName:         "role-1",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"subjects":[{"kind":"User","name":"bob@acme.com"},{"kind":"User","apiGroup":"rbac.authorization.k8s.io","name":"test@example.com"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 4: bind existing user",
			roleName:         "role-1",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"error":{"code":400,"message":"user test@example.com already connected to role role-1"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusBadRequest,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRoleBinding("test", "role-1", "test@example.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// group scenarios
		// scenario 4
		{
			name:             "scenario 5: bind group to role-1, when cluster role binding doesn't exist",
			roleName:         "role-1",
			body:             `{"group":"test"}`,
			expectedResponse: `{"error":{"code":500,"message":"the cluster role binding not found"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusInternalServerError,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 6: create cluster role binding when cluster role doesn't exist",
			roleName:         "role-1",
			body:             `{"group":"test"}`,
			expectedResponse: `{"error":{"code":404,"message":"clusterroles.rbac.authorization.k8s.io \"role-1\" not found"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusNotFound,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-2"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// scenario 7
		{
			name:             "scenario 7: update existing binding for the new group",
			roleName:         "role-1",
			body:             `{"group":"test"}`,
			expectedResponse: `{"subjects":[{"kind":"User","name":"bob@acme.com"},{"kind":"Group","apiGroup":"rbac.authorization.k8s.io","name":"test"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 8: update existing binding for the new group",
			roleName:         "role-1",
			body:             `{"group":"test"}`,
			expectedResponse: `{"subjects":[{"kind":"Group","name":"admins"},{"kind":"Group","apiGroup":"rbac.authorization.k8s.io","name":"test"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultGroupClusterRoleBinding("test", "role-1", "admins"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 9: bind existing group",
			roleName:         "role-1",
			body:             `{"group":"test"}`,
			expectedResponse: `{"error":{"code":400,"message":"group test already connected to role role-1"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusBadRequest,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultGroupClusterRoleBinding("test", "role-1", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 10: admin can update existing binding for the new user for any cluster",
			roleName:         "role-1",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"subjects":[{"kind":"User","name":"bob@acme.com"},{"kind":"User","apiGroup":"rbac.authorization.k8s.io","name":"test@example.com"}],"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", true),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
		{
			name:             "scenario 11: user John can not update existing binding for the new user for Bob's cluster",
			roleName:         "role-1",
			body:             `{"userEmail":"test@example.com"}`,
			expectedResponse: `{"error":{"code":403,"message":"forbidden: \"john@acme.com\" doesn't belong to the given project = my-first-project-ID"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusForbidden,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", false),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var kubernetesObj []runtime.Object
			var kubeObj []runtime.Object
			var kubermaticObj []runtime.Object
			kubeObj = append(kubeObj, tc.existingKubernrtesObjs...)
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/clusterroles/%s/clusterbindings", test.ProjectName, tc.clusterToGet, tc.roleName), strings.NewReader(tc.body))
			res := httptest.NewRecorder()

			kubermaticObj = append(kubermaticObj, tc.existingKubermaticObjs...)
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.existingAPIUser, nil, kubeObj, kubernetesObj, kubermaticObj, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.httpStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.httpStatus, res.Code, res.Body.String())
			}
			test.CompareWithResult(t, res, tc.expectedResponse)
		})
	}
}

func TestUnbindUserFromClusterRoleBinding(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name                   string
		body                   string
		roleName               string
		expectedResponse       string
		httpStatus             int
		clusterToGet           string
		existingAPIUser        *apiv1.User
		existingKubermaticObjs []runtime.Object
		existingKubernrtesObjs []runtime.Object
	}{
		// scenario 1
		{
			name:             "scenario 1: remove user from existing cluster role binding",
			roleName:         "role-1",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// scenario 2
		{
			name:             "scenario 2: remove group from existing cluster role binding",
			roleName:         "role-1",
			body:             `{"group":"test"}`,
			expectedResponse: `{"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultGroupClusterRoleBinding("test", "role-1", "test"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		{
			name:             "scenario 3: the admin can remove user from existing cluster role binding for any cluster",
			roleName:         "role-1",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"roleRefName":"role-1"}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", true),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
		{
			name:             "scenario 4: the user can not remove user from existing cluster role binding for Bob's cluster",
			roleName:         "role-1",
			body:             `{"userEmail":"bob@acme.com"}`,
			expectedResponse: `{"error":{"code":403,"message":"forbidden: \"john@acme.com\" doesn't belong to the given project = my-first-project-ID"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusForbidden,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", false),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("test", "role-1", "bob@acme.com"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var kubernetesObj []runtime.Object
			var kubeObj []runtime.Object
			var kubermaticObj []runtime.Object
			kubeObj = append(kubeObj, tc.existingKubernrtesObjs...)
			req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/clusterroles/%s/clusterbindings", test.ProjectName, tc.clusterToGet, tc.roleName), strings.NewReader(tc.body))
			res := httptest.NewRecorder()

			kubermaticObj = append(kubermaticObj, tc.existingKubermaticObjs...)
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.existingAPIUser, nil, kubeObj, kubernetesObj, kubermaticObj, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.httpStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.httpStatus, res.Code, res.Body.String())
			}
			test.CompareWithResult(t, res, tc.expectedResponse)
		})
	}
}

func TestListClusterRoleBinding(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name                   string
		expectedResponse       string
		httpStatus             int
		clusterToGet           string
		existingAPIUser        *apiv1.User
		existingKubermaticObjs []runtime.Object
		existingKubernrtesObjs []runtime.Object
	}{
		// scenario 1
		{
			name:             "scenario 1: list cluster role bindings",
			expectedResponse: `[{"subjects":[{"kind":"User","name":"test-1"}],"roleRefName":"role-1"},{"subjects":[{"kind":"User","name":"test-2"}],"roleRefName":"role-1"},{"subjects":[{"kind":"User","name":"test-3"}],"roleRefName":"role-2"},{"subjects":[{"kind":"Group","name":"test-4"}],"roleRefName":"role-2"}]`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("binding-1-1", "role-1", "test-1"),
				genDefaultClusterRoleBinding("binding-1-2", "role-1", "test-2"),
				genDefaultClusterRoleBinding("binding-2-1", "role-2", "test-3"),
				genDefaultGroupClusterRoleBinding("binding-2-2", "role-2", "test-4"),
			},
			existingAPIUser: test.GenDefaultAPIUser(),
		},
		// scenario 2
		{
			name:             "scenario 2: the admin John can list cluster role bindings for any cluster",
			expectedResponse: `[{"subjects":[{"kind":"User","name":"test-1"}],"roleRefName":"role-1"},{"subjects":[{"kind":"User","name":"test-2"}],"roleRefName":"role-1"},{"subjects":[{"kind":"User","name":"test-3"}],"roleRefName":"role-2"},{"subjects":[{"kind":"Group","name":"test-4"}],"roleRefName":"role-2"}]`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusOK,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", true),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("binding-1-1", "role-1", "test-1"),
				genDefaultClusterRoleBinding("binding-1-2", "role-1", "test-2"),
				genDefaultClusterRoleBinding("binding-2-1", "role-2", "test-3"),
				genDefaultGroupClusterRoleBinding("binding-2-2", "role-2", "test-4"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
		// scenario 3
		{
			name:             "scenario 3: the user John can not list Bob's cluster role bindings",
			expectedResponse: `{"error":{"code":403,"message":"forbidden: \"john@acme.com\" doesn't belong to the given project = my-first-project-ID"}}`,
			clusterToGet:     test.GenDefaultCluster().Name,
			httpStatus:       http.StatusForbidden,
			existingKubermaticObjs: test.GenDefaultKubermaticObjects(
				test.GenDefaultCluster(),
				genUser("John", "john@acme.com", false),
			),
			existingKubernrtesObjs: []runtime.Object{
				genDefaultClusterRole("role-1"),
				genDefaultClusterRole("role-2"),
				genDefaultClusterRoleBinding("binding-1-1", "role-1", "test-1"),
				genDefaultClusterRoleBinding("binding-1-2", "role-1", "test-2"),
				genDefaultClusterRoleBinding("binding-2-1", "role-2", "test-3"),
				genDefaultGroupClusterRoleBinding("binding-2-2", "role-2", "test-4"),
			},
			existingAPIUser: test.GenAPIUser("John", "john@acme.com"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var kubernetesObj []runtime.Object
			var kubeObj []runtime.Object
			var kubermaticObj []runtime.Object
			kubeObj = append(kubeObj, tc.existingKubernrtesObjs...)
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/projects/%s/dc/us-central1/clusters/%s/clusterbindings", test.ProjectName, tc.clusterToGet), strings.NewReader(""))
			res := httptest.NewRecorder()

			kubermaticObj = append(kubermaticObj, tc.existingKubermaticObjs...)
			ep, _, err := test.CreateTestEndpointAndGetClients(*tc.existingAPIUser, nil, kubeObj, kubernetesObj, kubermaticObj, nil, nil, hack.NewTestRouting)
			if err != nil {
				t.Fatalf("failed to create test endpoint due to %v", err)
			}

			ep.ServeHTTP(res, req)

			if res.Code != tc.httpStatus {
				t.Fatalf("Expected HTTP status code %d, got %d: %s", tc.httpStatus, res.Code, res.Body.String())
			}

			test.CompareWithResult(t, res, tc.expectedResponse)
		})
	}
}
