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
	kubermaticv1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"
	kuberneteshelper "github.com/kubermatic/kubermatic/pkg/kubernetes"
	kubermaticlog "github.com/kubermatic/kubermatic/pkg/log"
	"github.com/kubermatic/kubermatic/pkg/semver"
	"github.com/minio/minio-go/pkg/set"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimefakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockBackendOperations struct {
	localSnapshots    set.StringSet
	uploadedSnapshots set.StringSet
	returnError       error
}

func newMockBackendOperations() *mockBackendOperations {
	return &mockBackendOperations{
		localSnapshots:    set.NewStringSet(),
		uploadedSnapshots: set.NewStringSet(),
	}
}

func (ops *mockBackendOperations) takeSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string, cluster *kubermaticv1.Cluster) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	ops.localSnapshots.Add(fileName)
	return nil
}

func (ops *mockBackendOperations) uploadSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	if !ops.localSnapshots.Contains(fileName) {
		return fmt.Errorf("cannot upload non-existing local backup: %v", fileName)
	}
	ops.uploadedSnapshots.Add(fileName)
	return nil
}

func (ops *mockBackendOperations) cleanupSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	if !ops.localSnapshots.Contains(fileName) {
		return fmt.Errorf("cannot clean up non-existing local backup: %v", fileName)
	}
	ops.localSnapshots.Remove(fileName)
	return nil
}

func (ops *mockBackendOperations) deleteUploadedSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	ops.uploadedSnapshots.Remove(fileName)
	return nil
}

func genTestCluster() *kubermaticv1.Cluster {
	return &kubermaticv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testcluster",
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

func TestController_NonScheduled_SimpleBackup(t *testing.T) {
	cluster := genTestCluster()
	backup := genBackup(cluster, "testbackup")
	backupName := fmt.Sprintf("%s-%s", cluster.GetName(), backup.GetName())

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             &clock.RealClock{},
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

	if !reflect.DeepEqual(readbackBackup.Status.LastBackups, []string{backupName}) {
		t.Fatalf("backup not marked as completed")
	}

	if !kuberneteshelper.HasFinalizer(readbackBackup, DeleteAllBackupsFinalizer) {
		t.Fatalf("backup does not have finalizer %s", DeleteAllBackupsFinalizer)
	}
}

func TestController_NonScheduled_CompletedBackupIsNotProcessed(t *testing.T) {
	cluster := genTestCluster()

	backup := genBackup(cluster, "testbackup")
	backup.Status.LastBackups = []string{"testbackup"}
	backup.Status.LastBackupTime = &metav1.Time{Time: time.Now()}

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             &clock.RealClock{},
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

func TestController_cleanupBackup(t *testing.T) {
	cluster := genTestCluster()

	backup := genBackup(cluster, "testbackup")
	existingBackups := []string{"testcluster-backup1","testcluster-backup2","testcluster-backup3"}
	backup.Status.LastBackups = existingBackups
	kuberneteshelper.AddFinalizer(backup, DeleteAllBackupsFinalizer)

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             &clock.RealClock{},
	}

	for _, backupFileName := range existingBackups {
		mockBackOps.uploadedSnapshots.Add(backupFileName)
	}

	if err := reconciler.deleteAllBackups(context.Background(), reconciler.log, backup); err != nil {
		t.Fatalf("Error during deleteAllBackups call: %v", err)
	}

	if !mockBackOps.uploadedSnapshots.IsEmpty() {
		t.Fatalf("Expected no uploaded snapshots, got %v", mockBackOps.uploadedSnapshots)
	}

	readbackBackup := &kubermaticv1.EtcdBackup{}
	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backup.GetNamespace(), Name: backup.GetName()}, readbackBackup); err != nil {
		t.Fatalf("Error reading back completed backup: %v", err)
	}

	if len(backup.Finalizers) != 0 {
		t.Fatalf("Expected no remaining backup finalizers after cleanup call, got: %v", backup.Finalizers)
	}
}
//
//func TestController_BackupError(t *testing.T) {
//	cluster := genTestCluster()
//
//	const backupName = "testbackup"
//	backup := genBackup(cluster, backupName)
//
//	const errorMessage = "simulated error"
//
//	mockBackOps := newMockBackendOperations()
//	mockBackOps.returnError = fmt.Errorf(errorMessage)
//
//	eventRecorder := record.NewFakeRecorder(10)
//	reconciler := &Reconciler{
//		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
//		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
//		BackendOperations: mockBackOps,
//		recorder:          eventRecorder,
//		clock:             &clock.RealClock{},
//	}
//
//	_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backup.Namespace, Name: backup.Name}})
//
//	if !mockBackOps.localSnapshots.IsEmpty() {
//		t.Fatalf("expected no local snapshots, got: %v", mockBackOps.localSnapshots)
//	}
//
//	if !mockBackOps.uploadedSnapshots.IsEmpty() {
//		t.Fatalf("expected no uploaded snapshots, got: %v", mockBackOps.uploadedSnapshots)
//	}
//
//	if err == nil {
//		t.Fatal("Reconcile error expected")
//	}
//
//	if !strings.Contains(err.Error(), errorMessage) {
//		t.Fatalf("Expected error message containing '%v' but got %v", errorMessage, err)
//	}
//
//	events := collectEvents(eventRecorder.Events)
//	if len(events) != 2 {
//		t.Fatalf("Expected 2 events to be generated, got instead: %v", events)
//	}
//	for _, e := range events {
//		if !strings.Contains(e, errorMessage) {
//			t.Fatalf("Expected only events containing '%s' to be generated, got instead: %v", errorMessage, events)
//		}
//	}
//}
//
//func TestController_expireBackup(t *testing.T) {
//	cluster := genTestCluster()
//
//	const backupName = "testbackup"
//	backup := genBackup(cluster, backupName)
//	setBackupCondition(backup, kubermaticv1.EtcdBackupCreated, corev1.ConditionTrue)
//	kuberneteshelper.AddFinalizer(backup, DeleteAllBackupsFinalizer)
//	backup.Spec.TTL = &metav1.Duration{Duration: 10 * time.Minute}
//
//	clock := clock.NewFakeClock(time.Unix(100, 0))
//	backup.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
//
//	reconciler := &Reconciler{
//		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
//		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backup),
//		BackendOperations: newMockBackendOperations(),
//		recorder:          record.NewFakeRecorder(10),
//		clock:             clock,
//	}
//
//	readbackBackup := &kubermaticv1.EtcdBackup{}
//	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backup.GetNamespace(), Name: backup.GetName()}, readbackBackup); err != nil {
//		t.Fatalf("Expected to find backup, got an error: %v", err)
//	}
//
//	// sleep less than the TTL, check that backup is still there after reconciliation
//
//	preSleep := 1 * time.Minute
//	clock.Sleep(preSleep)
//
//	result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backup.Namespace, Name: backup.Name}})
//	if err != nil {
//		t.Fatalf("Error syncing cluster: %v", err)
//	}
//
//	if result.RequeueAfter != backup.Spec.TTL.Duration-preSleep {
//		t.Fatalf("Expected request to requeue after %v, but got %v", backup.Spec.TTL.Duration-preSleep, result.RequeueAfter)
//	}
//
//	readbackBackup = &kubermaticv1.EtcdBackup{}
//	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backup.GetNamespace(), Name: backup.GetName()}, readbackBackup); err != nil {
//		t.Fatalf("Expected to find backup, got an error: %v", err)
//	}
//
//	// sleep beyond the TTL, check that backup will be deleted
//
//	clock.Sleep(backup.Spec.TTL.Duration)
//
//	if _, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backup.Namespace, Name: backup.Name}}); err != nil {
//		t.Fatalf("Error syncing cluster: %v", err)
//	}
//
//	readbackBackup = &kubermaticv1.EtcdBackup{}
//	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backup.GetNamespace(), Name: backup.GetName()}, readbackBackup); !errors.IsNotFound(err) {
//		t.Fatalf("Expected backup to be deleted, got: backup=%v, err=%v", readbackBackup, err)
//	}
//}
//
//func collectEvents(source <-chan string) []string {
//	done := false
//	events := make([]string, 0)
//	for !done {
//		select {
//		case event := <-source:
//			events = append(events, event)
//		default:
//			done = true
//		}
//	}
//	return events
//}
