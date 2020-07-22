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

package backup

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/pkg/set"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	kubermaticv1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"
	kubermaticlog "github.com/kubermatic/kubermatic/pkg/log"
	"github.com/kubermatic/kubermatic/pkg/semver"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimefakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockBackendOperations struct {
	localSnapshots    set.StringSet
	uploadedSnapshots set.StringSet
}

func newMockBackendOperations() *mockBackendOperations {
	return &mockBackendOperations{
		localSnapshots:    set.NewStringSet(),
		uploadedSnapshots: set.NewStringSet(),
	}
}

func (ops *mockBackendOperations) takeSnapshot(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	ops.localSnapshots.Add(backup.GetName())
	return nil
}

func (ops *mockBackendOperations) uploadSnapshot(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	if !ops.localSnapshots.Contains(backup.GetName()) {
		return fmt.Errorf("cannot upload non-existing local backup: %v", backup.GetName())
	}
	ops.uploadedSnapshots.Add(backup.GetName())
	return nil
}

func (ops *mockBackendOperations) cleanupSnapshot(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	if !ops.localSnapshots.Contains(backup.GetName()) {
		return fmt.Errorf("cannot clean up non-existing local backup: %v", backup.GetName())
	}
	ops.localSnapshots.Remove(backup.GetName())
	return nil
}

func genTestCluster() *kubermaticv1.Cluster {
	return &kubermaticv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Spec: kubermaticv1.ClusterSpec{
			Version: *semver.NewSemverOrDie("1.16.3"),
		},
		Status: kubermaticv1.ClusterStatus{
			NamespaceName: "testnamespace",
			ExtendedHealth: kubermaticv1.ExtendedClusterHealth{
				Apiserver: kubermaticv1.HealthStatusUp,
			},
		},
	}
}

func genBackup(cluster *kubermaticv1.Cluster, name string) *kubermaticv1.EtcdBackup {
	return &kubermaticv1.EtcdBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Status.NamespaceName,
		},
		Spec: kubermaticv1.EtcdBackupSpec{
			Name: name,
		},
	}
}

func TestController_SimpleBackup(t *testing.T) {
	cluster := genTestCluster()

	const backupName = "testbackup"
	backup := genBackup(cluster, backupName)

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
		BackendOperations: mockBackOps,
	}

	if _, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backup.Namespace, Name: backup.Name}}); err != nil {
		t.Fatalf("Error syncing cluster: %v", err)
	}

	if !mockBackOps.uploadedSnapshots.Contains(backupName) {
		t.Fatalf("backup %v wasn't uploaded", backupName)
	}

	if !mockBackOps.localSnapshots.IsEmpty() {
		t.Fatalf("local snapshots weren't cleaned up, remaining: %v", mockBackOps.localSnapshots)
	}

	readbackBackup := &kubermaticv1.EtcdBackup{}
	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backup.GetNamespace(), Name: backup.GetName()}, readbackBackup); err != nil {
		t.Fatalf("Error reading back completed backup: %v", err)
	}

	if !readbackBackup.Status.HasConditionValue(kubermaticv1.EtcdBackupCreated, corev1.ConditionTrue) {
		t.Fatalf("backup not marked as completed")
	}
}

func TestController_CompletedBackupIsNotProcessed(t *testing.T) {
	cluster := genTestCluster()

	const backupName = "testbackup"
	backup := genBackup(cluster, backupName)
	setBackupCondition(backup, kubermaticv1.EtcdBackupCreated, corev1.ConditionTrue)

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
		BackendOperations: mockBackOps,
	}

	if _, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backup.Namespace, Name: backup.Name}}); err != nil {
		t.Fatalf("Error syncing cluster: %v", err)
	}

	if !mockBackOps.uploadedSnapshots.IsEmpty() {
		t.Fatalf("Expected no uploaded snapshots, got %v", mockBackOps.uploadedSnapshots)
	}

	if !mockBackOps.localSnapshots.IsEmpty() {
		t.Fatalf("Expected no local snapshots, got %v", mockBackOps.uploadedSnapshots)
	}
}
