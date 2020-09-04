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

package etcdbackup

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	kuberneteshelper "k8c.io/kubermatic/v2/pkg/kubernetes"
	kubermaticlog "k8c.io/kubermatic/v2/pkg/log"
	"k8c.io/kubermatic/v2/pkg/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimefakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type mockBackendOperations struct {
	localSnapshots    sets.String
	uploadedSnapshots sets.String
	returnError       error
}

func newMockBackendOperations() *mockBackendOperations {
	return &mockBackendOperations{
		localSnapshots:    sets.NewString(),
		uploadedSnapshots: sets.NewString(),
	}
}

func (ops *mockBackendOperations) takeSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string, cluster *kubermaticv1.Cluster) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	ops.localSnapshots.Insert(fileName)
	return nil
}

func (ops *mockBackendOperations) uploadSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	if !ops.localSnapshots.Has(fileName) {
		return fmt.Errorf("cannot upload non-existing local backup: %v", fileName)
	}
	ops.uploadedSnapshots.Insert(fileName)
	return nil
}

func (ops *mockBackendOperations) deleteSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	if !ops.localSnapshots.Has(fileName) {
		return fmt.Errorf("cannot clean up non-existing local backup: %v", fileName)
	}
	ops.localSnapshots.Delete(fileName)
	return nil
}

func (ops *mockBackendOperations) deleteUploadedSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error {
	if ops.returnError != nil {
		return ops.returnError
	}
	ops.uploadedSnapshots.Delete(fileName)
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

func genBackupConfig(cluster *kubermaticv1.Cluster, name string) *kubermaticv1.EtcdBackupConfig {
	return &kubermaticv1.EtcdBackupConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Status.NamespaceName,
		},
		Spec: kubermaticv1.EtcdBackupConfigSpec{
			Name: name,
			Cluster: corev1.ObjectReference{
				Kind: kubermaticv1.ClusterKindName,
				Name: cluster.GetName(),
			},
		},
	}
}

func TestController_NonScheduled_SimpleBackup(t *testing.T) {
	cluster := genTestCluster()
	backupConfig := genBackupConfig(cluster, "testbackup")
	backupName := fmt.Sprintf("%s-%s", cluster.GetName(), backupConfig.GetName())

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             &clock.RealClock{},
	}

	backupConfig, _ = mustReconcile(t, reconciler, backupConfig)

	if !mockBackOps.uploadedSnapshots.Has(backupName) {
		t.Fatalf("backupConfig %v wasn't uploaded", backupName)
	}

	if mockBackOps.localSnapshots.Len() != 0 {
		t.Fatalf("local snapshots weren't cleaned up, remaining: %v", mockBackOps.localSnapshots)
	}

	if !reflect.DeepEqual(backupConfig.Status.CurrentBackups, []string{backupName}) {
		t.Fatalf("created backup not added to .Status.CurrentBackups")
	}

	if backupConfig.Status.LastBackupTime == nil {
		t.Fatalf("no .Status.LastBackupTime recorded")
	}

	if !kuberneteshelper.HasFinalizer(backupConfig, DeleteAllBackupsFinalizer) {
		t.Fatalf("backupConfig does not have finalizer %s", DeleteAllBackupsFinalizer)
	}
}

func TestController_NonScheduled_CompletedBackupIsNotProcessed(t *testing.T) {
	cluster := genTestCluster()

	backupConfig := genBackupConfig(cluster, "testbackup")
	backupConfig.Status.CurrentBackups = []string{"testbackup"}
	backupConfig.Status.LastBackupTime = &metav1.Time{Time: time.Now()}

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             &clock.RealClock{},
	}

	_, _ = mustReconcile(t, reconciler, backupConfig)

	if mockBackOps.uploadedSnapshots.Len() != 0 {
		t.Fatalf("Expected no uploaded snapshots, got %v", mockBackOps.uploadedSnapshots)
	}

	if mockBackOps.localSnapshots.Len() != 0 {
		t.Fatalf("Expected no local snapshots, got %v", mockBackOps.uploadedSnapshots)
	}
}

func TestController_NonScheduled_cleanupBackup(t *testing.T) {
	cluster := genTestCluster()

	backupConfig := genBackupConfig(cluster, "testbackup")
	existingBackups := []string{"testcluster-backup1", "testcluster-backup2", "testcluster-backup3"}
	backupConfig.Status.CurrentBackups = existingBackups
	kuberneteshelper.AddFinalizer(backupConfig, DeleteAllBackupsFinalizer)

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             &clock.RealClock{},
	}

	for _, backupFileName := range existingBackups {
		mockBackOps.uploadedSnapshots.Insert(backupFileName)
	}

	if err := reconciler.deleteAllBackups(context.Background(), reconciler.log, backupConfig); err != nil {
		t.Fatalf("Error during deleteAllBackups call: %v", err)
	}

	if mockBackOps.uploadedSnapshots.Len() != 0 {
		t.Fatalf("Expected no uploaded snapshots, got %v", mockBackOps.uploadedSnapshots)
	}

	readbackBackup := &kubermaticv1.EtcdBackupConfig{}
	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackup); err != nil {
		t.Fatalf("Error reading back completed backupConfig: %v", err)
	}

	if len(backupConfig.Status.CurrentBackups) != 0 {
		t.Fatalf("Expected no remaining backups after cleanup call, got: %v", backupConfig.Status.CurrentBackups)
	}

	if len(backupConfig.Finalizers) != 0 {
		t.Fatalf("Expected no remaining backupConfig finalizers after cleanup call, got: %v", backupConfig.Finalizers)
	}
}

func TestController_BackupError(t *testing.T) {
	cluster := genTestCluster()

	backupConfig := genBackupConfig(cluster, "testbackup")

	const errorMessage = "simulated error"

	mockBackOps := newMockBackendOperations()
	mockBackOps.returnError = fmt.Errorf(errorMessage)

	eventRecorder := record.NewFakeRecorder(10)
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		BackendOperations: mockBackOps,
		recorder:          eventRecorder,
		clock:             &clock.RealClock{},
	}

	_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backupConfig.Namespace, Name: backupConfig.Name}})

	if mockBackOps.localSnapshots.Len() != 0 {
		t.Fatalf("expected no local snapshots, got: %v", mockBackOps.localSnapshots)
	}

	if mockBackOps.uploadedSnapshots.Len() != 0 {
		t.Fatalf("expected no uploaded snapshots, got: %v", mockBackOps.uploadedSnapshots)
	}

	if err == nil {
		t.Fatal("Reconcile error expected")
	}

	if !strings.Contains(err.Error(), errorMessage) {
		t.Fatalf("Expected error message containing '%v' but got %v", errorMessage, err)
	}

	events := collectEvents(eventRecorder.Events)
	if len(events) != 2 {
		t.Fatalf("Expected 2 events to be generated, got instead: %v", events)
	}
	for _, e := range events {
		if !strings.Contains(e, errorMessage) {
			t.Fatalf("Expected only events containing '%s' to be generated, got instead: %v", errorMessage, events)
		}
	}
}

func TestController_Scheduled(t *testing.T) {
	cluster := genTestCluster()

	backupConfig := genBackupConfig(cluster, "testbackup")

	clock := clock.NewFakeClock(time.Unix(0, 0))
	backupConfig.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
	// back up every 10 minutes, keep 2 backups
	backupConfig.Spec.Schedule = "*/10 * * * *"
	backupConfig.Spec.Keep = intPtr(2)

	mockBackOps := newMockBackendOperations()
	reconciler := &Reconciler{
		log:               kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:            ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		BackendOperations: mockBackOps,
		recorder:          record.NewFakeRecorder(10),
		clock:             clock,
	}

	// sleep to before the first 10 minute backupConfig, check that no backupConfig is created

	preSleep := 1 * time.Minute
	clock.Sleep(preSleep)

	backupConfig, result := mustReconcile(t, reconciler, backupConfig)

	if result.RequeueAfter != 10*time.Minute-preSleep {
		t.Fatalf("Expected request to requeue after %v, but got %v", 10*time.Minute-preSleep, result.RequeueAfter)
	}

	if mockBackOps.uploadedSnapshots.Len() != 0 {
		t.Fatalf("no uploaded snapshots expected, got: %v", mockBackOps.uploadedSnapshots)
	}

	// sleep beyond the first backupConfig time, check that a backupConfig is created

	clock.Sleep(10 * time.Minute)

	backupConfig, _ = mustReconcile(t, reconciler, backupConfig)

	expectedBackups := []string{}

	expectedBackups = append(expectedBackups, backupName(backupConfig, cluster, clock.Now()))

	if !reflect.DeepEqual(backupConfig.Status.CurrentBackups, expectedBackups) {
		t.Fatalf(".status.currentBackups expected: %v, got: %v", expectedBackups, backupConfig.Status.CurrentBackups)
	}
	if !stringSetContainsExactly(mockBackOps.uploadedSnapshots, expectedBackups...) {
		t.Fatalf("uploaded snapshots expected: %v, got: %v", expectedBackups, mockBackOps.uploadedSnapshots)
	}

	// sleep beyond the second backupConfig time, check that a backupConfig is created

	clock.Sleep(10 * time.Minute)

	backupConfig, _ = mustReconcile(t, reconciler, backupConfig)

	expectedBackups = append(expectedBackups, backupName(backupConfig, cluster, clock.Now()))

	if !reflect.DeepEqual(backupConfig.Status.CurrentBackups, expectedBackups) {
		t.Fatalf(".status.currentBackups expected: %v, got: %v", expectedBackups, backupConfig.Status.CurrentBackups)
	}
	if !stringSetContainsExactly(mockBackOps.uploadedSnapshots, expectedBackups...) {
		t.Fatalf("uploaded snapshots expected: %v, got: %v", expectedBackups, mockBackOps.uploadedSnapshots)
	}

	// sleep beyond the third backupConfig time, check that a backupConfig is created and the first one is deleted

	clock.Sleep(10 * time.Minute)

	backupConfig, _ = mustReconcile(t, reconciler, backupConfig)

	expectedBackups = append(expectedBackups, backupName(backupConfig, cluster, clock.Now()))
	expectedBackups = expectedBackups[1:]

	if !reflect.DeepEqual(backupConfig.Status.CurrentBackups, expectedBackups) {
		t.Fatalf(".status.currentBackups expected: %v, got: %v", expectedBackups, backupConfig.Status.CurrentBackups)
	}
	if !stringSetContainsExactly(mockBackOps.uploadedSnapshots, expectedBackups...) {
		t.Fatalf("uploaded snapshots expected: %v, got: %v", expectedBackups, mockBackOps.uploadedSnapshots)
	}
}

func mustReconcile(t *testing.T, reconciler *Reconciler, backupConfig *kubermaticv1.EtcdBackupConfig) (*kubermaticv1.EtcdBackupConfig, reconcile.Result) {
	var result reconcile.Result
	var err error
	if result, err = reconciler.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: backupConfig.Namespace, Name: backupConfig.Name}}); err != nil {
		t.Fatalf("Error syncing cluster: %v", err)
	}

	readbackBackupConfig := &kubermaticv1.EtcdBackupConfig{}
	if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackupConfig); err != nil {
		t.Fatalf("Error reading back reconciled backupConfig: %v", err)
	}

	return readbackBackupConfig, result
}

func backupName(backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster, time time.Time) string {
	return fmt.Sprintf("%s-%s-%s", cluster.GetName(), backupConfig.GetName(), time.Format("2006-01-02T15:04:05"))
}

func intPtr(i int) *int {
	return &i
}

func stringSetContainsExactly(set sets.String, elements ...string) bool {
	return set.Len() == len(elements) && set.HasAll(elements...)
}

func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case event := <-source:
			events = append(events, event)
		default:
			done = true
		}
	}
	return events
}
