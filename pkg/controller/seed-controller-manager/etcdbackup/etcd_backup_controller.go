/*
Copyright 2020 The Kubermatic Kubernetes Platform contributors.

Copyright 2017 the Velero contributors. (func parseCronSchedule)

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
	"github.com/robfig/cron"
	"go.uber.org/zap"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	kubermaticv1helper "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1/helper"
	kuberneteshelper "k8c.io/kubermatic/v2/pkg/kubernetes"
	errors2 "k8c.io/kubermatic/v2/pkg/util/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/record"
	"reflect"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	ControllerName     = "kubermatic_etcd_backup_controller"
	defaultClusterSize = 3

	// DeleteAllBackupsFinalizer indicates that the backups still need to be deleted in the backend
	DeleteAllBackupsFinalizer = "kubermatic.io/delete-all-backups"
)

type BackendOperations interface {
	takeSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string, cluster *kubermaticv1.Cluster) error
	uploadSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error
	deleteSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error
	deleteUploadedSnapshot(ctx context.Context, log *zap.SugaredLogger, fileName string) error
}

// Reconciler stores necessary components that are required to create etcd backups
type Reconciler struct {
	log        *zap.SugaredLogger
	workerName string
	ctrlruntimeclient.Client
	BackendOperations
	clock    clock.Clock
	recorder record.EventRecorder
}

// Add creates a new Backup controller that is responsible for
// managing cluster etcd backups
func Add(
	mgr manager.Manager,
	log *zap.SugaredLogger,
	numWorkers int,
	workerName string,
	snapshotDir string,
	s3Endpoint string,
	s3BucketName string,
	s3AccessKeyID string,
	s3SecretAccessKey string,
) error {
	log = log.Named(ControllerName)
	client := mgr.GetClient()

	reconciler := &Reconciler{
		log:        log,
		Client:     client,
		workerName: workerName,
		recorder:   mgr.GetEventRecorderFor(ControllerName),
		BackendOperations: &s3BackendOperations{
			snapshotDir:       snapshotDir,
			s3Endpoint:        s3Endpoint,
			s3BucketName:      s3BucketName,
			s3AccessKeyID:     s3AccessKeyID,
			s3SecretAccessKey: s3SecretAccessKey,
		},
		clock: &clock.RealClock{},
	}

	ctrlOptions := controller.Options{
		Reconciler:              reconciler,
		MaxConcurrentReconciles: numWorkers,
	}
	c, err := controller.New(ControllerName, mgr, ctrlOptions)
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &kubermaticv1.EtcdBackup{}}, &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := r.log.With("request", request)
	log.Debug("Processing")

	backup := &kubermaticv1.EtcdBackup{}
	if err := r.Get(ctx, request.NamespacedName, backup); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	cluster := &kubermaticv1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.Cluster.Name}, cluster); err != nil {
		return reconcile.Result{}, err
	}

	log = r.log.With("cluster", cluster.Name, "backup", backup.Name)

	// Add a wrapping here so we can emit an event on error
	result, err := kubermaticv1helper.ClusterReconcileWrapper(
		ctx,
		r.Client,
		r.workerName,
		cluster,
		kubermaticv1.ClusterConditionBackupControllerReconcilingSuccess,
		func() (*reconcile.Result, error) {
			return r.reconcile(ctx, log, backup, cluster)
		},
	)
	if err != nil {
		log.Errorw("Reconciling failed", zap.Error(err))
		r.recorder.Event(backup, corev1.EventTypeWarning, "ReconcilingError", err.Error())
		r.recorder.Eventf(cluster, corev1.EventTypeWarning, "ReconcilingError",
			"failed to reconcile etcd backup %q: %v", backup.Name, err)
	}
	if result == nil {
		result = &reconcile.Result{}
	}
	return *result, err
}

func (r *Reconciler) reconcile(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	if backup.DeletionTimestamp != nil {
		log.Debug("Cleaning up all backups")
		return nil, wrapErrorMessage("error cleaning up all backups: %v", r.deleteAllBackups(ctx, log, backup))
	}

	backupName, err := r.currentlyPendingBackupName(backup, cluster)
	if err != nil {
		return nil, fmt.Errorf("can't determine backup schedule: %v", err)
	}

	if backupName != "" {
		err := r.createBackup(ctx, log, backupName, backup, cluster)
		if err != nil {
			return nil, fmt.Errorf("error creating backup: %v", err)
		}
	}

	if err := r.deleteBackupsUpToRemaining(ctx, log, backup, backup.GetKeptBackupsCount()); err != nil {
		return nil, fmt.Errorf("error expiring old backups: %v", err)
	}

	reconcile, err := r.computeReconcileAfter(backup)
	if err != nil {
		// should not happen at this point because the schedule was already parsed successfully above
		return nil, fmt.Errorf("error computing reconcile interval: %v", err)
	}

	return reconcile, nil
}

// return name of backup to be done right now, or "" if no backup needs to be done right now
func (r *Reconciler) currentlyPendingBackupName(backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) (string, error) {
	prefix := backupFileNamePrefix(backup.Name, cluster.Name)

	if backup.Spec.Schedule == "" {
		// no schedule set => we need exactly one backup (if none was created yet)
		if backup.Status.LastBackupTime == nil {
			return prefix, nil
		}
		return "", nil
	}

	schedule, err := parseCronSchedule(backup.Spec.Schedule)
	if err != nil {
		return "", err
	}

	lastBackupTime := backup.Status.LastBackupTime
	if lastBackupTime == nil {
		lastBackupTime = &metav1.Time{Time: backup.CreationTimestamp.Time}
	}

	if r.clock.Now().After(schedule.Next(lastBackupTime.Time)) {
		return fmt.Sprintf("%s-%s", prefix, r.clock.Now().Format("2006-01-02T15:04:05")), nil
	}

	return "", nil
}

func (r *Reconciler) computeReconcileAfter(backup *kubermaticv1.EtcdBackup) (*reconcile.Result, error) {
	if backup.Spec.Schedule == "" {
		// no schedule set => only one immediate backup, which is already created at this point => all done, no need to reschedule
		return nil, nil
	}

	schedule, err := parseCronSchedule(backup.Spec.Schedule)
	if err != nil {
		return nil, err
	}

	lastBackupTime := backup.Status.LastBackupTime
	if lastBackupTime == nil {
		lastBackupTime = &metav1.Time{Time: r.clock.Now()}
	}

	durationToNextBackup := schedule.Next(lastBackupTime.Time).Sub(r.clock.Now())
	if durationToNextBackup < 0 {
		durationToNextBackup = 0
	}
	return &reconcile.Result{Requeue: true, RequeueAfter: durationToNextBackup}, nil
}

func (r *Reconciler) createBackup(ctx context.Context, log *zap.SugaredLogger, fileName string, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	err := r.takeSnapshot(ctx, log, fileName, cluster)
	if err != nil {
		return fmt.Errorf("error taking snapshot: %v", err)
	}

	defer func() {
		if err := r.deleteSnapshot(ctx, log, fileName); err != nil {
			log.Errorf("Failed to delete snapshot: %v", err)
		}
	}()

	err = r.uploadSnapshot(ctx, log, fileName)
	if err != nil {
		return fmt.Errorf("error uploading snapshot: %v", err)
	}

	return wrapErrorMessage(
		"failed to modify EtcdBackup resource: %v",
		r.updateBackup(ctx, backup, func(backup *kubermaticv1.EtcdBackup) {
			kuberneteshelper.AddFinalizer(backup, DeleteAllBackupsFinalizer)
			backup.Status.LastBackupTime = &metav1.Time{Time: r.clock.Now()}
			backup.Status.CurrentBackups = append(backup.Status.CurrentBackups, fileName)
		}))
}

func (r *Reconciler) deleteBackupsUpToRemaining(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, remaining int) error {
	for len(backup.Status.CurrentBackups) > remaining {
		toDelete := backup.Status.CurrentBackups[0]
		if err := r.deleteUploadedSnapshot(ctx, log, toDelete); err != nil {
			// TODO ignore not-found errors
			return fmt.Errorf("error deleting uploaded snapshot %v: %v", toDelete, err)
		}
		err := r.updateBackup(ctx, backup, func(backup *kubermaticv1.EtcdBackup) {
			backup.Status.CurrentBackups = backup.Status.CurrentBackups[1:]
		})
		if err != nil {
			return fmt.Errorf("failed to update EtcdBackup after deleting backup %v: %v", toDelete, err)
		}
	}
	return nil
}

func (r *Reconciler) deleteAllBackups(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup) error {
	if !kuberneteshelper.HasFinalizer(backup, DeleteAllBackupsFinalizer) {
		return nil
	}

	err := r.deleteBackupsUpToRemaining(ctx, log, backup, 0)
	if err != nil {
		return err
	}

	return wrapErrorMessage("error removing finalizer: %v", r.updateBackup(ctx, backup, func(backup *kubermaticv1.EtcdBackup) {
		kuberneteshelper.RemoveFinalizer(backup, DeleteAllBackupsFinalizer)
	}))
}

func (r *Reconciler) updateBackup(ctx context.Context, backup *kubermaticv1.EtcdBackup, modify func(*kubermaticv1.EtcdBackup)) error {
	oldBackup := backup.DeepCopy()
	modify(backup)
	if reflect.DeepEqual(oldBackup, backup) {
		return nil
	}
	return r.Client.Patch(ctx, backup, ctrlruntimeclient.MergeFrom(oldBackup))
}

func backupFileNamePrefix(backupName, clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, backupName)
}

func parseCronSchedule(scheduleString string) (cron.Schedule, error) {
	var validationErrors []error
	var schedule cron.Schedule

	// cron.Parse panics if schedule is empty
	if len(scheduleString) == 0 {
		return nil, fmt.Errorf("Schedule must be a non-empty valid Cron expression")
	}

	// adding a recover() around cron.Parse because it panics on empty string and is possible
	// that it panics under other scenarios as well.
	func() {
		defer func() {
			if r := recover(); r != nil {
				validationErrors = append(validationErrors, fmt.Errorf("(panic) invalid schedule: %v", r))
			}
		}()

		if res, err := cron.ParseStandard(scheduleString); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid schedule: %v", err))
		} else {
			schedule = res
		}
	}()

	if len(validationErrors) > 0 {
		return nil, errors2.NewAggregate(validationErrors)
	}

	return schedule, nil
}

func wrapErrorMessage(wrapMessage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(wrapMessage, err)
}
