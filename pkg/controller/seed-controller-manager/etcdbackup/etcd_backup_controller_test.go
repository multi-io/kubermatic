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
	"github.com/go-test/deep"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	kubermaticlog "k8c.io/kubermatic/v2/pkg/log"
	"k8c.io/kubermatic/v2/pkg/semver"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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

func genStoreContainer() *corev1.Container {
	return &corev1.Container{
		Name:  "test-store-container",
		Image: "some-s3cmd:latest",
		Command: []string{
			"/bin/sh",
			"-c",
			"s3cmd ...",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "FOO",
				Value: "xx",
			},
			{
				Name:  "BAR",
				Value: "yy",
			},
		},
	}
}

func genDeleteContainer() *corev1.Container {
	return &corev1.Container{
		Name:  "test-delete-container",
		Image: "some-s3cmd:latest",
		Command: []string{
			"/bin/sh",
			"-c",
			"s3cmd ...",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "FOO",
				Value: "xx",
			},
			{
				Name:  "BAR",
				Value: "yy",
			},
		},
	}
}

func genBackupJob(backupName string, jobName string) *batchv1.Job {
	// jerry-rig a cluster, BackupConfig and BackupStatus instance to create a job object
	// that's similar to the ones an actual reconciliation will create
	cluster := genTestCluster()
	backupConfig := genBackupConfig(cluster, "testbackup")
	backup := &kubermaticv1.BackupStatus{
		BackupName: backupName,
		JobName:    jobName,
	}
	reconciler := Reconciler{
		log:            kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:         ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		storeContainer: genStoreContainer(),
		recorder:       record.NewFakeRecorder(10),
		clock:          clock.RealClock{},
	}
	job := reconciler.backupJob(backupConfig, cluster, backup)
	job.ResourceVersion = "1"
	return job
}

func genDeleteJob(backupName string, jobName string) *batchv1.Job {
	// same thing as genBackupJob, but for delete jobs
	cluster := genTestCluster()
	backupConfig := genBackupConfig(cluster, "testbackup")
	backup := &kubermaticv1.BackupStatus{
		BackupName:    backupName,
		DeleteJobName: jobName,
	}
	reconciler := Reconciler{
		log:             kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
		Client:          ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
		deleteContainer: genDeleteContainer(),
		recorder:        record.NewFakeRecorder(10),
		clock:           clock.RealClock{},
	}
	job := reconciler.deleteJob(backupConfig, cluster, backup)
	job.ResourceVersion = "1"
	return job
}

func jobAddCondition(j *batchv1.Job, jobType batchv1.JobConditionType, status corev1.ConditionStatus, lastTransitionTime time.Time, message string) *batchv1.Job {
	j.Status.Conditions = append(j.Status.Conditions, batchv1.JobCondition{
		Type:               jobType,
		Status:             status,
		LastTransitionTime: metav1.Time{Time: lastTransitionTime},
		Message:            message,
	})
	return j
}

func TestEnsureNextBackupIsScheduled(t *testing.T) {
	testCases := []struct {
		name              string
		currentTime       time.Time
		schedule          string
		existingBackups   []kubermaticv1.BackupStatus
		expectedBackups   []kubermaticv1.BackupStatus
		expectedReconcile *reconcile.Result
	}{
		{
			name:            "scheduling on a no-schedule Config with no backups schedules one one-shot backup immediately",
			currentTime:     time.Unix(10, 0).UTC(),
			schedule:        "",
			existingBackups: []kubermaticv1.BackupStatus{},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(10, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:00:10",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedReconcile: &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 0,
			},
		},
		{
			name:        "scheduling on a no-schedule Config with a scheduled backup doesn't change anything",
			currentTime: time.Unix(10, 0).UTC(),
			schedule:    "",
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(100, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:01:40",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(100, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:01:40",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedReconcile: &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 90 * time.Second,
			},
		},
		{
			name:            "scheduling on a Config with no backups schedules one backup",
			currentTime:     time.Unix(10, 0).UTC(),
			schedule:        "*/10 * * * *",
			existingBackups: []kubermaticv1.BackupStatus{},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedReconcile: &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 590 * time.Second,
			},
		},
		{
			name:        "scheduling on a Config with future backups already scheduled changes nothing",
			currentTime: time.Unix(10, 0).UTC(), //
			schedule:    "*/10 * * * *",
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedReconcile: &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 590 * time.Second,
			},
		},
		{
			name:        "scheduling on a Config with no future backups scheduled schedules the next one",
			currentTime: time.Unix(700, 0).UTC(),
			schedule:    "*/10 * * * *",
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(1200, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:20:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedReconcile: &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 500 * time.Second,
			},
		},
		{
			name:        "scheduling on a Config with a future backups scheduled for a time not matching the schedule schedules a new backup",
			currentTime: time.Unix(700, 0).UTC(),
			schedule:    "*/10 * * * *",
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
				{
					// this could only happen if you change the .schedule field of a BackupConfig later.
					ScheduledTime: &metav1.Time{Time: time.Unix(720, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:12:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(600, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:10:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(720, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:12:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(1200, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:20:00",
					JobName:       "testcluster-backup-testbackup-create-xxxx",
					DeleteJobName: "testcluster-backup-testbackup-delete-xxxx",
				},
			},
			expectedReconcile: &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 500 * time.Second,
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cluster := genTestCluster()
			backupConfig := genBackupConfig(cluster, "testbackup")

			clock := clock.NewFakeClock(tc.currentTime.UTC())
			backupConfig.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
			backupConfig.Spec.Schedule = tc.schedule
			backupConfig.Status.CurrentBackups = tc.existingBackups

			reconciler := Reconciler{
				log:                 kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
				Client:              ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, cluster, backupConfig),
				recorder:            record.NewFakeRecorder(10),
				clock:               clock,
				randStringGenerator: constRandStringGenerator("xxxx"),
			}

			reconcileAfter, err := reconciler.ensureNextBackupIsScheduled(context.Background(), backupConfig, cluster)
			if err != nil {
				t.Fatalf("ensureNextBackupIsScheduled returned an error: %v", err)
			}

			readbackBackupConfig := &kubermaticv1.EtcdBackupConfig{}
			if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackupConfig); err != nil {
				t.Fatalf("Error reading back completed backupConfig: %v", err)
			}

			if diff := deep.Equal(backupConfig.Status, readbackBackupConfig.Status); diff != nil {
				t.Errorf("backupsConfig status differs from read back one, diff: %v", diff)
			}

			if diff := deep.Equal(readbackBackupConfig.Status.CurrentBackups, tc.expectedBackups); diff != nil {
				t.Errorf("backups differ from expected, diff: %v", diff)
			}

			if deep.Equal(reconcileAfter, tc.expectedReconcile) != nil {
				t.Errorf("reconcile time differs from expected, expected: %v, actual: %v", tc.expectedReconcile, reconcileAfter)
			}
		})
	}
}

func TestStartPendingBackupJobs(t *testing.T) {
	testCases := []struct {
		name              string
		currentTime       time.Time
		existingBackups   []kubermaticv1.BackupStatus
		existingJobs      []batchv1.Job
		expectedBackups   []kubermaticv1.BackupStatus
		expectedReconcile *reconcile.Result
		expectedJobs      []batchv1.Job
	}{
		{
			name:        "backup job scheduled in the past it started, job scheduled in the future is not",
			currentTime: time.Unix(90, 0).UTC(),
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:       "testcluster-backup-testbackup-create-aaaa",
					DeleteJobName: "testcluster-backup-testbackup-delete-aaaa",
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:       "testcluster-backup-testbackup-create-bbbb",
					DeleteJobName: "testcluster-backup-testbackup-delete-bbbb",
				},
			},
			existingJobs: []batchv1.Job{},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:       "testcluster-backup-testbackup-create-aaaa",
					DeleteJobName: "testcluster-backup-testbackup-delete-aaaa",
					BackupPhase:   kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:       "testcluster-backup-testbackup-create-bbbb",
					DeleteJobName: "testcluster-backup-testbackup-delete-bbbb",
				},
			},
			expectedReconcile: &reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedJobs: []batchv1.Job{
				*genBackupJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-create-aaaa"),
			},
		},
		{
			name:        "finished backup job is marked as finished in the backup status",
			currentTime: time.Unix(90, 0).UTC(),
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:       "testcluster-backup-testbackup-create-aaaa",
					DeleteJobName: "testcluster-backup-testbackup-delete-aaaa",
					BackupPhase:   kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(70, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:01:10",
					JobName:       "testcluster-backup-testbackup-create-bbbb",
					DeleteJobName: "testcluster-backup-testbackup-delete-bbbb",
					BackupPhase:   kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			existingJobs: []batchv1.Job{
				*jobAddCondition(genBackupJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-create-aaaa"),
					batchv1.JobComplete, corev1.ConditionTrue, time.Unix(90, 0).UTC(), "job completed"),
				*jobAddCondition(genBackupJob("testcluster-testbackup-1970-01-01T00:01:10", "testcluster-backup-testbackup-create-bbbb"),
					batchv1.JobFailed, corev1.ConditionTrue, time.Unix(80, 0).UTC(), "Job has reached the specified backoff limit"),
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(70, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:10",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(80, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseFailed,
					BackupMessage:      "Job has reached the specified backoff limit",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseCompleted,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(80, 0).UTC()},
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			expectedReconcile: &reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedJobs: []batchv1.Job{
				*jobAddCondition(genBackupJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-create-aaaa"),
					batchv1.JobComplete, corev1.ConditionTrue, time.Unix(90, 0).UTC(), "job completed"),
				*jobAddCondition(genBackupJob("testcluster-testbackup-1970-01-01T00:01:10", "testcluster-backup-testbackup-create-bbbb"),
					batchv1.JobFailed, corev1.ConditionTrue, time.Unix(80, 0).UTC(), "Job has reached the specified backoff limit"),
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cluster := genTestCluster()
			backupConfig := genBackupConfig(cluster, "testbackup")

			clock := clock.NewFakeClock(tc.currentTime.UTC())
			backupConfig.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
			backupConfig.Status.CurrentBackups = tc.existingBackups

			initObjs := []runtime.Object{
				cluster,
				backupConfig,
			}
			for _, j := range tc.existingJobs {
				initObjs = append(initObjs, j.DeepCopy())
			}
			reconciler := Reconciler{
				log:            kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
				Client:         ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, initObjs...),
				storeContainer: genStoreContainer(),
				recorder:       record.NewFakeRecorder(10),
				clock:          clock,
			}

			reconcileAfter, err := reconciler.startPendingBackupJobs(context.Background(), backupConfig, cluster)
			if err != nil {
				t.Fatalf("ensureNextBackupIsScheduled returned an error: %v", err)
			}

			readbackBackupConfig := &kubermaticv1.EtcdBackupConfig{}
			if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackupConfig); err != nil {
				t.Fatalf("Error reading back completed backupConfig: %v", err)
			}

			if diff := deep.Equal(backupConfig.Status, readbackBackupConfig.Status); diff != nil {
				t.Errorf("backupsConfig status differs from read back one, diff: %v", diff)
			}

			if diff := deep.Equal(readbackBackupConfig.Status.CurrentBackups, tc.expectedBackups); diff != nil {
				t.Errorf("backups differ from expected, diff: %v", diff)
			}

			jobs := batchv1.JobList{}
			if err := reconciler.List(context.Background(), &jobs); err != nil {
				t.Fatalf("Error reading created jobs: %v", err)
			}
			if diff := deep.Equal(jobs.Items, tc.expectedJobs); diff != nil {
				t.Errorf("jobs differ from expected ones: %v", diff)
			}

			if deep.Equal(reconcileAfter, tc.expectedReconcile) != nil {
				t.Errorf("reconcile time differs from expected, expected: %v, actual: %v", tc.expectedReconcile, reconcileAfter)
			}
		})
	}
}

func TestStartPendingBackupDeleteJobs(t *testing.T) {
	testCases := []struct {
		name              string
		currentTime       time.Time
		keep              int
		existingBackups   []kubermaticv1.BackupStatus
		existingJobs      []batchv1.Job
		expectedBackups   []kubermaticv1.BackupStatus
		expectedReconcile *reconcile.Result
		expectedJobs      []batchv1.Job
	}{
		{
			name:        "delete job for completed backup is started",
			currentTime: time.Unix(170, 0).UTC(),
			keep:        1,
			existingBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(180, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			existingJobs: []batchv1.Job{},
			expectedBackups: []kubermaticv1.BackupStatus{
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(180, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			expectedReconcile: &reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedJobs: []batchv1.Job{
				*genDeleteJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-delete-aaaa"),
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cluster := genTestCluster()
			backupConfig := genBackupConfig(cluster, "testbackup")

			clock := clock.NewFakeClock(tc.currentTime.UTC())
			backupConfig.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
			backupConfig.Spec.Schedule = "xxx" // must be non-empty
			backupConfig.Spec.Keep = intPtr(tc.keep)
			backupConfig.Status.CurrentBackups = tc.existingBackups

			initObjs := []runtime.Object{
				cluster,
				backupConfig,
			}
			for _, j := range tc.existingJobs {
				initObjs = append(initObjs, j.DeepCopy())
			}
			reconciler := Reconciler{
				log:             kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
				Client:          ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, initObjs...),
				deleteContainer: genDeleteContainer(),
				recorder:        record.NewFakeRecorder(10),
				clock:           clock,
			}

			reconcileAfter, err := reconciler.startPendingBackupDeleteJobs(context.Background(), backupConfig, cluster)
			if err != nil {
				t.Fatalf("ensureNextBackupIsScheduled returned an error: %v", err)
			}

			readbackBackupConfig := &kubermaticv1.EtcdBackupConfig{}
			if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackupConfig); err != nil {
				t.Fatalf("Error reading back completed backupConfig: %v", err)
			}

			if diff := deep.Equal(backupConfig.Status, readbackBackupConfig.Status); diff != nil {
				t.Errorf("backupsConfig status differs from read back one, diff: %v", diff)
			}

			if diff := deep.Equal(readbackBackupConfig.Status.CurrentBackups, tc.expectedBackups); diff != nil {
				t.Errorf("backups differ from expected, diff: %v", diff)
			}

			jobs := batchv1.JobList{}
			if err := reconciler.List(context.Background(), &jobs); err != nil {
				t.Fatalf("Error reading created jobs: %v", err)
			}
			if diff := deep.Equal(jobs.Items, tc.expectedJobs); diff != nil {
				t.Errorf("jobs differ from expected ones: %v", diff)
			}

			if deep.Equal(reconcileAfter, tc.expectedReconcile) != nil {
				t.Errorf("reconcile time differs from expected, expected: %v, actual: %v", tc.expectedReconcile, reconcileAfter)
			}
		})
	}
}

func TestUpdateRunningBackupDeleteJobs(t *testing.T) {
	testCases := []struct {
		name              string
		currentTime       time.Time
		existingBackups   []kubermaticv1.BackupStatus
		existingJobs      []batchv1.Job
		expectedBackups   []kubermaticv1.BackupStatus
		expectedReconcile *reconcile.Result
	}{
		{
			name:        "deletion is marked as complete if corresponding job has completed",
			currentTime: time.Unix(170, 0).UTC(),
			existingBackups: []kubermaticv1.BackupStatus{
				// 3 backups with deletions marked as running, a 4th backup is only scheduled
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(180, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:03:00",
					JobName:            "testcluster-backup-testbackup-create-cccc",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(210, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-cccc",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(240, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:04:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			existingJobs: []batchv1.Job{
				// first backup's deletion job succeeded, second one's failed, third one's is still running
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-delete-aaaa"),
					batchv1.JobComplete, corev1.ConditionTrue, time.Unix(100, 0).UTC(), "job completed"),
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:02:00", "testcluster-backup-testbackup-delete-bbbb"),
					batchv1.JobFailed, corev1.ConditionTrue, time.Unix(160, 0).UTC(), "job failed"),
				*genDeleteJob("testcluster-testbackup-1970-01-01T00:03:00", "testcluster-backup-testbackup-delete-cccc"),
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				// result: 1st backup's deletion marked as completed, 2nd one's marked as failed, 3rd and 4th unchanged
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
					DeletePhase:        kubermaticv1.BackupStatusPhaseCompleted,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(100, 0).UTC()},
					DeleteMessage:      "job completed",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseFailed,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(160, 0).UTC()},
					DeleteMessage:      "job failed",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(180, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:03:00",
					JobName:            "testcluster-backup-testbackup-create-cccc",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(210, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-cccc",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(240, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:04:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			expectedReconcile: &reconcile.Result{RequeueAfter: 30 * time.Second},
		},
		{
			name:        "if all backup deletions are marked as completed, nothing is changed and we reconcile after the job retention time",
			currentTime: time.Unix(170, 0).UTC(),
			existingBackups: []kubermaticv1.BackupStatus{
				// 2 backups with deletions marked as running
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
			},
			existingJobs: []batchv1.Job{
				// both backup's deletion jobs ended
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-delete-aaaa"),
					batchv1.JobComplete, corev1.ConditionTrue, time.Unix(100, 0).UTC(), "job completed"),
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:02:00", "testcluster-backup-testbackup-delete-bbbb"),
					batchv1.JobComplete, corev1.ConditionTrue, time.Unix(160, 0).UTC(), "job completed"),
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				// result: both backups' deletions marked as completed, and we reconcile after the retention period
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
					DeletePhase:        kubermaticv1.BackupStatusPhaseCompleted,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(100, 0).UTC()},
					DeleteMessage:      "job completed",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseCompleted,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(160, 0).UTC()},
					DeleteMessage:      "job completed",
				},
			},
			expectedReconcile: &reconcile.Result{RequeueAfter: deletedBackupJobRetentionTime},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cluster := genTestCluster()
			backupConfig := genBackupConfig(cluster, "testbackup")

			clock := clock.NewFakeClock(tc.currentTime.UTC())
			backupConfig.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
			backupConfig.Status.CurrentBackups = tc.existingBackups

			initObjs := []runtime.Object{
				cluster,
				backupConfig,
			}
			for _, j := range tc.existingJobs {
				initObjs = append(initObjs, j.DeepCopy())
			}
			reconciler := Reconciler{
				log:             kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
				Client:          ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, initObjs...),
				deleteContainer: genDeleteContainer(),
				recorder:        record.NewFakeRecorder(10),
				clock:           clock,
			}

			reconcileAfter, err := reconciler.updateRunningBackupDeleteJobs(context.Background(), backupConfig, cluster)
			if err != nil {
				t.Fatalf("ensureNextBackupIsScheduled returned an error: %v", err)
			}

			readbackBackupConfig := &kubermaticv1.EtcdBackupConfig{}
			if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackupConfig); err != nil {
				t.Fatalf("Error reading back completed backupConfig: %v", err)
			}

			if diff := deep.Equal(backupConfig.Status, readbackBackupConfig.Status); diff != nil {
				t.Errorf("backupsConfig status differs from read back one, diff: %v", diff)
			}

			if diff := deep.Equal(readbackBackupConfig.Status.CurrentBackups, tc.expectedBackups); diff != nil {
				t.Errorf("backups differ from expected, diff: %v", diff)
			}

			if deep.Equal(reconcileAfter, tc.expectedReconcile) != nil {
				t.Errorf("reconcile time differs from expected, expected: %v, actual: %v", tc.expectedReconcile, reconcileAfter)
			}
		})
	}
}

func TestDeleteFinishedBackupJobs(t *testing.T) {
	testCases := []struct {
		name              string
		currentTime       time.Time
		existingBackups   []kubermaticv1.BackupStatus
		existingJobs      []batchv1.Job
		expectedBackups   []kubermaticv1.BackupStatus
		expectedReconcile *reconcile.Result
		expectedJobs      []batchv1.Job
	}{
		{
			name:        "deletion is marked as complete if corresponding job has completed",
			existingBackups: []kubermaticv1.BackupStatus{
				// 2 backups with deletions marked as completed, one with deletion marked as running, a 4th backup is only scheduled
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(60, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:01:00",
					JobName:            "testcluster-backup-testbackup-create-aaaa",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(90, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-aaaa",
					DeletePhase:        kubermaticv1.BackupStatusPhaseCompleted,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(100, 0).UTC()},
					DeleteMessage:      "job completed",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseFailed,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(160, 0).UTC()},
					DeleteMessage:      "job failed",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(180, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:03:00",
					JobName:            "testcluster-backup-testbackup-create-cccc",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(210, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-cccc",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(240, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:04:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			// current time is such that the first backup's deletion time is past the retention time but the others aren't
			currentTime: time.Unix(120, 0).Add(deletedBackupJobRetentionTime).UTC(),
			existingJobs: []batchv1.Job{
				// first backup's deletion job succeeded, second one's failed, third one's is still running
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:01:00", "testcluster-backup-testbackup-delete-aaaa"),
					batchv1.JobComplete, corev1.ConditionTrue, time.Unix(100, 0).UTC(), "job completed"),
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:02:00", "testcluster-backup-testbackup-delete-bbbb"),
					batchv1.JobFailed, corev1.ConditionTrue, time.Unix(160, 0).UTC(), "job failed"),
				*genDeleteJob("testcluster-testbackup-1970-01-01T00:03:00", "testcluster-backup-testbackup-delete-cccc"),
			},
			expectedBackups: []kubermaticv1.BackupStatus{
				// result: 1st backup's job and status entry are deleted, rest unchanged
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(120, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:02:00",
					JobName:            "testcluster-backup-testbackup-create-bbbb",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(150, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-bbbb",
					DeletePhase:        kubermaticv1.BackupStatusPhaseFailed,
					DeleteFinishedTime: &metav1.Time{Time: time.Unix(160, 0).UTC()},
					DeleteMessage:      "job failed",
				},
				{
					ScheduledTime:      &metav1.Time{Time: time.Unix(180, 0).UTC()},
					BackupName:         "testcluster-testbackup-1970-01-01T00:03:00",
					JobName:            "testcluster-backup-testbackup-create-cccc",
					BackupFinishedTime: &metav1.Time{Time: time.Unix(210, 0).UTC()},
					BackupPhase:        kubermaticv1.BackupStatusPhaseCompleted,
					BackupMessage:      "job completed",
					DeleteJobName:      "testcluster-backup-testbackup-delete-cccc",
					DeletePhase:        kubermaticv1.BackupStatusPhaseRunning,
				},
				{
					ScheduledTime: &metav1.Time{Time: time.Unix(240, 0).UTC()},
					BackupName:    "testcluster-testbackup-1970-01-01T00:04:00",
					JobName:       "testcluster-backup-testbackup-create-cccc",
					DeleteJobName: "testcluster-backup-testbackup-delete-cccc",
				},
			},
			expectedJobs: []batchv1.Job{
				*jobAddCondition(genDeleteJob("testcluster-testbackup-1970-01-01T00:02:00", "testcluster-backup-testbackup-delete-bbbb"),
					batchv1.JobFailed, corev1.ConditionTrue, time.Unix(160, 0).UTC(), "job failed"),
				*genDeleteJob("testcluster-testbackup-1970-01-01T00:03:00", "testcluster-backup-testbackup-delete-cccc"),
			},
			// reconcile when the 2nd backup's retention time runs out
			expectedReconcile: &reconcile.Result{RequeueAfter: 40 * time.Second},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cluster := genTestCluster()
			backupConfig := genBackupConfig(cluster, "testbackup")

			clock := clock.NewFakeClock(tc.currentTime.UTC())
			backupConfig.SetCreationTimestamp(metav1.Time{Time: clock.Now()})
			backupConfig.Status.CurrentBackups = tc.existingBackups

			initObjs := []runtime.Object{
				cluster,
				backupConfig,
			}
			for _, j := range tc.existingJobs {
				initObjs = append(initObjs, j.DeepCopy())
			}
			reconciler := Reconciler{
				log:             kubermaticlog.New(true, kubermaticlog.FormatConsole).Sugar(),
				Client:          ctrlruntimefakeclient.NewFakeClientWithScheme(scheme.Scheme, initObjs...),
				deleteContainer: genDeleteContainer(),
				recorder:        record.NewFakeRecorder(10),
				clock:           clock,
			}

			reconcileAfter, err := reconciler.deleteFinishedBackupJobs(context.Background(), backupConfig, cluster)
			if err != nil {
				t.Fatalf("ensureNextBackupIsScheduled returned an error: %v", err)
			}

			readbackBackupConfig := &kubermaticv1.EtcdBackupConfig{}
			if err := reconciler.Get(context.Background(), client.ObjectKey{Namespace: backupConfig.GetNamespace(), Name: backupConfig.GetName()}, readbackBackupConfig); err != nil {
				t.Fatalf("Error reading back completed backupConfig: %v", err)
			}

			if diff := deep.Equal(backupConfig.Status, readbackBackupConfig.Status); diff != nil {
				t.Errorf("backupsConfig status differs from read back one, diff: %v", diff)
			}

			if diff := deep.Equal(readbackBackupConfig.Status.CurrentBackups, tc.expectedBackups); diff != nil {
				t.Errorf("backups differ from expected, diff: %v", diff)
			}

			jobs := batchv1.JobList{}
			if err := reconciler.List(context.Background(), &jobs); err != nil {
				t.Fatalf("Error reading created jobs: %v", err)
			}
			if diff := deep.Equal(jobs.Items, tc.expectedJobs); diff != nil {
				t.Errorf("jobs differ from expected ones: %v", diff)
			}

			if deep.Equal(reconcileAfter, tc.expectedReconcile) != nil {
				t.Errorf("reconcile time differs from expected, expected: %v, actual: %v", tc.expectedReconcile, reconcileAfter)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func constRandStringGenerator(str string) func() string {
	return func() string {
		return str
	}
}
