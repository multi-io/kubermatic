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
	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"go.uber.org/zap"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	kubermaticv1helper "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1/helper"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/certificates"
	"k8c.io/kubermatic/v2/pkg/resources/certificates/triple"
	"k8c.io/kubermatic/v2/pkg/resources/etcd"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"
	errors2 "k8c.io/kubermatic/v2/pkg/util/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	"reflect"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
	"strings"
	"time"
)

const (
	ControllerName     = "kubermatic_etcd_backup_controller"
	defaultClusterSize = 3

	// DeleteAllBackupsFinalizer indicates that the backups still need to be deleted in the backend
	DeleteAllBackupsFinalizer = "kubermatic.io/delete-all-backups"

	// BackupConfigNameLabelKey is the label key which should be used to name the BackupConfig a job belongs to
	BackupConfigNameLabelKey = "backupConfig"
	// DefaultBackupContainerImage holds the default Image used for creating the etcd backups
	DefaultBackupContainerImage = "gcr.io/etcd-development/etcd"
	// SharedVolumeName is the name of the `emptyDir` volume the initContainer
	// will write the backup to
	SharedVolumeName = "etcd-backup"
	// backupJobLabel defines the label we use on all backup jobs
	backupJobLabel = "kubermatic-etcd-backup"
	// clusterEnvVarKey defines the environment variable key for the cluster name
	clusterEnvVarKey = "CLUSTER"
	// backupToCreateEnvVarKey defines the environment variable key for the name of the backup to create
	backupToCreateEnvVarKey = "BACKUP_TO_CREATE"
	// backupToDeleteEnvVarKey defines the environment variable key for the name of the backup to delete
	backupToDeleteEnvVarKey = "BACKUP_TO_DELETE"

	// deletedBackupJobRetentionTime specifies how long we keep job resources and backupconfig.status.currentBackups
	// entries around after the backup processing has finished (incl. deletion)
	deletedBackupJobRetentionTime = 10 * time.Minute
)

// Reconciler stores necessary components that are required to create etcd backups
type Reconciler struct {
	log        *zap.SugaredLogger
	workerName string
	ctrlruntimeclient.Client
	storeContainer  *corev1.Container
	deleteContainer *corev1.Container
	// backupContainerImage holds the image used for creating the etcd backup
	// It must be configurable to cover offline use cases
	backupContainerImage string
	clock                clock.Clock
	randStringGenerator  func() string
	recorder             record.EventRecorder
}

// Add creates a new Backup controller that is responsible for
// managing cluster etcd backups
func Add(
	mgr manager.Manager,
	log *zap.SugaredLogger,
	numWorkers int,
	workerName string,
	storeContainer *corev1.Container,
	deleteContainer *corev1.Container,
	backupContainerImage string,
) error {
	log = log.Named(ControllerName)
	client := mgr.GetClient()

	if backupContainerImage == "" {
		backupContainerImage = DefaultBackupContainerImage
	}

	reconciler := &Reconciler{
		log:                  log,
		Client:               client,
		workerName:           workerName,
		storeContainer:       storeContainer,
		deleteContainer:      deleteContainer,
		backupContainerImage: backupContainerImage,
		recorder:             mgr.GetEventRecorderFor(ControllerName),
		clock:                &clock.RealClock{},
		randStringGenerator: func() string {
			return rand.String(10)
		},
	}

	ctrlOptions := controller.Options{
		Reconciler:              reconciler,
		MaxConcurrentReconciles: numWorkers,
	}
	c, err := controller.New(ControllerName, mgr, ctrlOptions)
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &kubermaticv1.EtcdBackupConfig{}}, &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := r.log.With("request", request)
	log.Debug("Processing")

	backupConfig := &kubermaticv1.EtcdBackupConfig{}
	if err := r.Get(ctx, request.NamespacedName, backupConfig); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	cluster := &kubermaticv1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: backupConfig.Spec.Cluster.Name}, cluster); err != nil {
		return reconcile.Result{}, err
	}

	log = r.log.With("cluster", cluster.Name, "backupConfig", backupConfig.Name)

	// Add a wrapping here so we can emit an event on error
	result, err := kubermaticv1helper.ClusterReconcileWrapper(
		ctx,
		r.Client,
		r.workerName,
		cluster,
		kubermaticv1.ClusterConditionBackupControllerReconcilingSuccess,
		func() (*reconcile.Result, error) {
			return r.reconcile(ctx, log, backupConfig, cluster)
		},
	)
	if err != nil {
		log.Errorw("Reconciling failed", zap.Error(err))
		r.recorder.Event(backupConfig, corev1.EventTypeWarning, "ReconcilingError", err.Error())
		r.recorder.Eventf(cluster, corev1.EventTypeWarning, "ReconcilingError",
			"failed to reconcile etcd backup config %q: %v", backupConfig.Name, err)
	}
	if result == nil {
		result = &reconcile.Result{}
	}
	return *result, err
}

func (r *Reconciler) reconcile(ctx context.Context, log *zap.SugaredLogger, backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	if backupConfig.DeletionTimestamp != nil {
		log.Debug("Cleaning up all backups")
		// TODO
		return nil, nil
	}

	if err := r.ensureEtcdClientSecret(ctx, cluster); err != nil {
		return nil, errors.Wrap(err, "failed to create backup secret")
	}

	var nextReconcile, totalReconcile *reconcile.Result
	var err error
	errorReconcile := &reconcile.Result{RequeueAfter: 1 * time.Minute}

	if nextReconcile, err = r.ensureNextBackupIsScheduled(ctx, backupConfig, cluster); err != nil {
		return errorReconcile, errors.Wrap(err, "failed to ensure next backup is scheduled")
	}

	totalReconcile = minReconcile(totalReconcile, nextReconcile)

	if nextReconcile, err = r.startPendingBackupJobs(ctx, backupConfig, cluster); err != nil {
		return errorReconcile, errors.Wrap(err, "failed to start pending and update running backups")
	}

	totalReconcile = minReconcile(totalReconcile, nextReconcile)

	if nextReconcile, err = r.startPendingBackupDeleteJobs(ctx, backupConfig, cluster); err != nil {
		return errorReconcile, errors.Wrap(err, "failed to start pending backup delete jobs")
	}

	totalReconcile = minReconcile(totalReconcile, nextReconcile)

	if nextReconcile, err = r.updateRunningBackupDeleteJobs(ctx, backupConfig, cluster); err != nil {
		return errorReconcile, errors.Wrap(err, "failed to update running backup delete jobs")
	}

	totalReconcile = minReconcile(totalReconcile, nextReconcile)

	if nextReconcile, err = r.deleteFinishedBackupJobs(ctx, backupConfig, cluster); err != nil {
		return errorReconcile, errors.Wrap(err, "failed to delete finished backup jobs")
	}

	totalReconcile = minReconcile(totalReconcile, nextReconcile)

	return totalReconcile, nil
}

func minReconcile(reconciles ...*reconcile.Result) *reconcile.Result {
	var result *reconcile.Result
	for _, r := range reconciles {
		if result == nil || (r != nil && r.RequeueAfter < result.RequeueAfter) {
			result = r
		}
	}
	return result
}

// ensure a backup is scheduled for the next time slot after the current time, according to the backup config's schedule.
// "schedule a backup" means set the scheduled time, backup name and job names of the corresponding element of backupConfig.Status.CurrentBackups
func (r *Reconciler) ensureNextBackupIsScheduled(ctx context.Context, backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	var backupToSchedule *kubermaticv1.BackupStatus

	oldBackupConfig := backupConfig.DeepCopy()

	if backupConfig.Spec.Schedule == "" {
		// no schedule set => we need to schedule exactly one backup (if none was scheduled yet)
		if len(backupConfig.Status.CurrentBackups) > 0 {
			// backups scheduled; just take CurrentBackups[0] and wait for it
			durationToScheduledTime := backupConfig.Status.CurrentBackups[0].ScheduledTime.Sub(r.clock.Now())
			if durationToScheduledTime >= 0 {
				return &reconcile.Result{Requeue: true, RequeueAfter: durationToScheduledTime}, nil
			} else {
				return nil, nil
			}
		} else {
			backupConfig.Status.CurrentBackups = []kubermaticv1.BackupStatus{{}}
			backupToSchedule = &backupConfig.Status.CurrentBackups[0]
			backupToSchedule.ScheduledTime = &metav1.Time{Time: r.clock.Now()}
		}

	} else {
		schedule, err := parseCronSchedule(backupConfig.Spec.Schedule)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to Parse Schedule %v", backupConfig.Spec.Schedule)
		}

		nextScheduledTime := schedule.Next(r.clock.Now())

		for _, backup := range backupConfig.Status.CurrentBackups {
			if backup.ScheduledTime.Time == nextScheduledTime {
				// found a scheduled backup for the next time slot. Nothing to change, requeue when its time arrives.
				return &reconcile.Result{Requeue: true, RequeueAfter: nextScheduledTime.Sub(r.clock.Now())}, nil
			}
		}

		backupConfig.Status.CurrentBackups = append(backupConfig.Status.CurrentBackups, kubermaticv1.BackupStatus{})
		backupToSchedule = &backupConfig.Status.CurrentBackups[len(backupConfig.Status.CurrentBackups)-1]
		backupToSchedule.ScheduledTime = &metav1.Time{Time: nextScheduledTime}
	}

	backupToSchedule.BackupName = fmt.Sprintf("%s-%s-%s", cluster.Name, backupConfig.Name, backupToSchedule.ScheduledTime.Format("2006-01-02T15:04:05"))
	backupToSchedule.JobName = fmt.Sprintf("%s-backup-%s-create-%s", cluster.Name, backupConfig.Name, r.randStringGenerator())
	backupToSchedule.DeleteJobName = fmt.Sprintf("%s-backup-%s-delete-%s", cluster.Name, backupConfig.Name, r.randStringGenerator())

	if err := r.Patch(ctx, backupConfig, ctrlruntimeclient.MergeFrom(oldBackupConfig)); err != nil {
		return nil, errors.Wrap(err, "failed to update backup config")
	}

	durationToScheduledTime := backupToSchedule.ScheduledTime.Sub(r.clock.Now())
	return &reconcile.Result{Requeue: true, RequeueAfter: durationToScheduledTime}, nil
}

// create any backup jobs that can be created, i.e. that don't exist yet while their scheduled time has arrived
// also update status of backups whose jobs have completed
func (r *Reconciler) startPendingBackupJobs(ctx context.Context, backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	oldBackupConfig := backupConfig.DeepCopy()

	activeJobs := false

	for i := range backupConfig.Status.CurrentBackups {
		backup := &backupConfig.Status.CurrentBackups[i]
		if backup.BackupPhase != kubermaticv1.BackupStatusPhaseCompleted && backup.BackupPhase != kubermaticv1.BackupStatusPhaseFailed {
			activeJobs = true

			if backup.BackupPhase == kubermaticv1.BackupStatusPhaseRunning {
				job := &batchv1.Job{}
				err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: backup.JobName}, job)
				if err != nil {
					if !kerrors.IsNotFound(err) {
						return nil, errors.Wrapf(err, "error getting job for backup %s", backup.BackupName)
					}
					// job not found. Apparently deleted externally.
					backup.BackupPhase = kubermaticv1.BackupStatusPhaseFailed
					backup.BackupMessage = "backup job deleted externally"
				} else {
					if cond := getJobConditionIfTrue(job, batchv1.JobComplete); cond != nil {
						backup.BackupPhase = kubermaticv1.BackupStatusPhaseCompleted
						backup.BackupMessage = cond.Message
						backup.BackupFinishedTime = &cond.LastTransitionTime
					} else if cond := getJobConditionIfTrue(job, batchv1.JobFailed); cond != nil {
						backup.BackupPhase = kubermaticv1.BackupStatusPhaseFailed
						backup.BackupMessage = cond.Message
						backup.BackupFinishedTime = &cond.LastTransitionTime
						// if the backup failed, we don't start a delete job => mark deletion as completed immediately
						backup.DeletePhase = kubermaticv1.BackupStatusPhaseCompleted
						backup.DeleteFinishedTime = &cond.LastTransitionTime
					}
				}

			} else if backup.BackupPhase == "" && r.clock.Now().Sub(backup.ScheduledTime.Time) >= 0 {
				job := r.backupJob(backupConfig, cluster, backup)
				if err := r.Create(ctx, job); err != nil && !kerrors.IsAlreadyExists(err) {
					return nil, errors.Wrapf(err, "error creating job for backup %s", backup.BackupName)
				}
				backup.BackupPhase = kubermaticv1.BackupStatusPhaseRunning
			}
		}
	}

	if err := r.Patch(ctx, backupConfig, ctrlruntimeclient.MergeFrom(oldBackupConfig)); err != nil {
		return nil, errors.Wrap(err, "failed to update backup config")
	}

	if activeJobs {
		return &reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	} else {
		return nil, nil
	}
}

// create any backup delete jobs that can be created, i.e. for all completed backups older than the last backupConfig.GetKeptBackupsCount() ones.
func (r *Reconciler) startPendingBackupDeleteJobs(ctx context.Context, backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	if backupConfig.Spec.Schedule == "" {
		return nil, nil
	}

	// find all backups that have completed but whose delete job hasn't started yet
	var completedBackups []*kubermaticv1.BackupStatus
	for i := range backupConfig.Status.CurrentBackups {
		backup := &backupConfig.Status.CurrentBackups[i]
		if backup.BackupPhase == kubermaticv1.BackupStatusPhaseCompleted && backup.DeletePhase == "" {
			completedBackups = append(completedBackups, backup)
		}
	}

	toDeleteCount := len(completedBackups) - backupConfig.GetKeptBackupsCount()
	if toDeleteCount > 0 {
		oldBackupConfig := backupConfig.DeepCopy()

		sort.Slice(completedBackups, func(i, j int) bool {
			return completedBackups[i].ScheduledTime.Time.Before(completedBackups[j].ScheduledTime.Time)
		})
		for i := 0; i < toDeleteCount; i++ {
			backup := completedBackups[i]
			job := r.deleteJob(backupConfig, cluster, backup)
			if err := r.Create(ctx, job); err != nil && !kerrors.IsAlreadyExists(err) {
				return nil, errors.Wrapf(err, "error creating delete job for backup %s", backup.BackupName)
			}
			backup.DeletePhase = kubermaticv1.BackupStatusPhaseRunning
		}

		if err := r.Patch(ctx, backupConfig, ctrlruntimeclient.MergeFrom(oldBackupConfig)); err != nil {
			return nil, errors.Wrap(err, "failed to update backup config")
		}

		return &reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return nil, nil
}

// update status of all delete jobs that have completed and are still marked as running
func (r *Reconciler) updateRunningBackupDeleteJobs(ctx context.Context, backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	oldBackupConfig := backupConfig.DeepCopy()

	var returnReconcile *reconcile.Result

	for i := range backupConfig.Status.CurrentBackups {
		backup := &backupConfig.Status.CurrentBackups[i]
		if backup.DeletePhase == kubermaticv1.BackupStatusPhaseRunning {
			job := &batchv1.Job{}
			err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: backup.DeleteJobName}, job)
			if err != nil {
				if !kerrors.IsNotFound(err) {
					return nil, errors.Wrapf(err, "error getting delete job for backup %s", backup.BackupName)
				}
				// job not found. Apparently deleted externally.
				backup.DeletePhase = kubermaticv1.BackupStatusPhaseFailed
				backup.DeleteMessage = "backup job deleted externally"
			} else {
				if cond := getJobConditionIfTrue(job, batchv1.JobComplete); cond != nil {
					backup.DeletePhase = kubermaticv1.BackupStatusPhaseCompleted
					backup.DeleteMessage = cond.Message
					backup.DeleteFinishedTime = &cond.LastTransitionTime
					returnReconcile = minReconcile(returnReconcile, &reconcile.Result{RequeueAfter: deletedBackupJobRetentionTime})
				} else if cond := getJobConditionIfTrue(job, batchv1.JobFailed); cond != nil {
					backup.DeletePhase = kubermaticv1.BackupStatusPhaseFailed
					backup.DeleteMessage = cond.Message
					backup.DeleteFinishedTime = &cond.LastTransitionTime
					returnReconcile = minReconcile(returnReconcile, &reconcile.Result{RequeueAfter: deletedBackupJobRetentionTime})
				} else {
					// job still running
					returnReconcile = minReconcile(returnReconcile, &reconcile.Result{RequeueAfter: 30 * time.Second})
				}
			}
		}
	}

	if err := r.Patch(ctx, backupConfig, ctrlruntimeclient.MergeFrom(oldBackupConfig)); err != nil {
		return nil, errors.Wrap(err, "failed to update backup config")
	}

	return returnReconcile, nil
}

// for backups that have been finished (including deletion) for a while, delete all associated job resources as well as the backup status entry
func (r *Reconciler) deleteFinishedBackupJobs(ctx context.Context, backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	oldBackupConfig := backupConfig.DeepCopy()

	var returnReconcile *reconcile.Result

	var newBackups []kubermaticv1.BackupStatus
	for _, backup := range backupConfig.Status.CurrentBackups {
		if backup.DeleteFinishedTime != nil {
			age := r.clock.Now().Sub(backup.DeleteFinishedTime.Time)

			if age < deletedBackupJobRetentionTime {
				// don't delete the backup and job yet, but reconcile when the time has come to delete them
				returnReconcile = minReconcile(returnReconcile, &reconcile.Result{RequeueAfter: deletedBackupJobRetentionTime - age})

			} else {
				job := &batchv1.Job{}

				// delete backup job
				err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: backup.JobName}, job)
				if err == nil {
					if err := r.Delete(ctx, job); err != nil && !kerrors.IsNotFound(err) {
						return nil, errors.Wrapf(err, "backup %s: failed to delete backup job (%s)", backup.BackupName, backup.JobName)
					}
				} else if !kerrors.IsNotFound(err) {
					return nil, errors.Wrapf(err, "backup %s: failed to get backup job (%s)", backup.BackupName, backup.JobName)
				}

				err = r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: backup.DeleteJobName}, job)
				if err == nil {
					if err := r.Delete(ctx, job); err != nil && !kerrors.IsNotFound(err) {
						return nil, errors.Wrapf(err, "backup %s: failed to delete backup deletion job (%s)", backup.BackupName, backup.DeleteJobName)
					}
				} else if !kerrors.IsNotFound(err) {
					return nil, errors.Wrapf(err, "backup %s: failed to get backup deletion job (%s)", backup.BackupName, backup.DeleteJobName)
				}

				continue // delete backup from backupConfig.Status.CurrentBackups
			}
		}

		newBackups = append(newBackups, backup)
	}

	backupConfig.Status.CurrentBackups = newBackups

	if err := r.Patch(ctx, backupConfig, ctrlruntimeclient.MergeFrom(oldBackupConfig)); err != nil {
		return nil, errors.Wrap(err, "failed to update backup config")
	}

	return returnReconcile, nil
}

func (r *Reconciler) backupJob(backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster, backupStatus *kubermaticv1.BackupStatus) *batchv1.Job {
	storeContainer := r.storeContainer.DeepCopy()
	storeContainer.Env = append(storeContainer.Env, corev1.EnvVar{
		Name:  clusterEnvVarKey,
		Value: cluster.Name,
	})
	storeContainer.Env = append(storeContainer.Env, corev1.EnvVar{
		Name:  backupToCreateEnvVarKey,
		Value: backupStatus.BackupName,
	})
	job := r.jobBase(backupConfig, cluster, backupStatus.JobName)
	job.Spec.Template.Spec.Containers = []corev1.Container{*storeContainer}
	return job
}

func (r *Reconciler) deleteJob(backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster, backupStatus *kubermaticv1.BackupStatus) *batchv1.Job {
	deleteContainer := r.deleteContainer.DeepCopy()
	deleteContainer.Env = append(deleteContainer.Env, corev1.EnvVar{
		Name:  clusterEnvVarKey,
		Value: cluster.Name,
	})
	deleteContainer.Env = append(deleteContainer.Env, corev1.EnvVar{
		Name:  backupToDeleteEnvVarKey,
		Value: backupStatus.BackupName,
	})
	job := r.jobBase(backupConfig, cluster, backupStatus.DeleteJobName)
	job.Spec.Template.Spec.Containers = []corev1.Container{*deleteContainer}
	return job
}

func (r *Reconciler) jobBase(backupConfig *kubermaticv1.EtcdBackupConfig, cluster *kubermaticv1.Cluster, jobName string) *batchv1.Job {
	endpoints := etcd.GetClientEndpoints(cluster.Status.NamespaceName)
	image := r.backupContainerImage
	if !strings.Contains(image, ":") {
		image = image + ":" + etcd.ImageTag(cluster)
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: metav1.NamespaceSystem,
			Labels: map[string]string{
				resources.AppLabelKey:    backupJobLabel,
				BackupConfigNameLabelKey: backupConfig.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				resources.GetClusterRef(cluster),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          utilpointer.Int32Ptr(3),
			Completions:           utilpointer.Int32Ptr(1),
			Parallelism:           utilpointer.Int32Ptr(1),
			ActiveDeadlineSeconds: resources.Int64(2 * 60),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					InitContainers: []corev1.Container{
						{
							Name:  "backup-creator",
							Image: image,
							Env: []corev1.EnvVar{
								{
									Name:  "ETCDCTL_API",
									Value: "3",
								},
								{
									Name:  "ETCDCTL_DIAL_TIMEOUT",
									Value: "3s",
								},
								{
									Name:  "ETCDCTL_CACERT",
									Value: "/etc/etcd/client/ca.crt",
								},
								{
									Name:  "ETCDCTL_CERT",
									Value: "/etc/etcd/client/backup-etcd-client.crt",
								},
								{
									Name:  "ETCDCTL_KEY",
									Value: "/etc/etcd/client/backup-etcd-client.key",
								},
							},
							Command: snapshotCommand(endpoints),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      SharedVolumeName,
									MountPath: "/backup",
								},
								{
									Name:      r.getEtcdSecretName(cluster),
									MountPath: "/etc/etcd/client",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: SharedVolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: r.getEtcdSecretName(cluster),
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: r.getEtcdSecretName(cluster),
								},
							},
						},
					},
				},
			},
		},
	}
}

func snapshotCommand(etcdEndpoints []string) []string {
	cmd := []string{
		"/bin/sh",
		"-c",
	}
	script := &strings.Builder{}
	// Accordings to its godoc, this always returns a nil error
	_, _ = script.WriteString(
		`backupOrReportFailure() {
  echo "Creating backup"
  if ! eval $@; then
    echo "Backup creation failed"
    return 1
  fi
  echo "Successfully created backup, exiting"
  exit 0
}`)
	for _, endpoint := range etcdEndpoints {
		_, _ = script.WriteString(fmt.Sprintf("\nbackupOrReportFailure etcdctl --endpoints %s snapshot save /backup/snapshot.db", endpoint))
	}
	_, _ = script.WriteString("\necho \"Unable to create backup\"\nexit 1")
	cmd = append(cmd, script.String())
	return cmd
}

func (r *Reconciler) getEtcdSecretName(cluster *kubermaticv1.Cluster) string {
	return fmt.Sprintf("cluster-%s-etcd-client-certificate", cluster.Name)
}

func (r *Reconciler) ensureEtcdClientSecret(ctx context.Context, cluster *kubermaticv1.Cluster) error {
	secretName := r.getEtcdSecretName(cluster)

	getCA := func() (*triple.KeyPair, error) {
		return resources.GetClusterRootCA(ctx, cluster.Status.NamespaceName, r.Client)
	}

	_, creator := certificates.GetClientCertificateCreator(
		secretName,
		"backup",
		nil,
		resources.BackupEtcdClientCertificateCertSecretKey,
		resources.BackupEtcdClientCertificateKeySecretKey,
		getCA,
	)()

	wrappedCreator := reconciling.SecretObjectWrapper(creator)
	wrappedCreator = reconciling.OwnerRefWrapper(resources.GetClusterRef(cluster))(wrappedCreator)

	err := reconciling.EnsureNamedObject(
		ctx,
		types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: secretName},
		wrappedCreator, r.Client, &corev1.Secret{}, false)
	if err != nil {
		return fmt.Errorf("failed to ensure Secret %q: %v", secretName, err)
	}

	return nil
}

func jobConditionSet(job *batchv1.Job, condType batchv1.JobConditionType) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == condType {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *Reconciler) updateBackupConfig(ctx context.Context, backupConfig *kubermaticv1.EtcdBackupConfig, modify func(*kubermaticv1.EtcdBackupConfig)) error {
	oldBackup := backupConfig.DeepCopy()
	modify(backupConfig)
	if reflect.DeepEqual(oldBackup, backupConfig) {
		return nil
	}
	return r.Client.Patch(ctx, backupConfig, ctrlruntimeclient.MergeFrom(oldBackup))
}

func backupFileNamePrefix(backupConfigName, clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, backupConfigName)
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

func getJobConditionIfTrue(job *batchv1.Job, condType batchv1.JobConditionType) *batchv1.JobCondition {
	if len(job.Status.Conditions) == 0 {
		return nil
	} else {
		for _, cond := range job.Status.Conditions {
			if cond.Type == condType && cond.Status == corev1.ConditionTrue {
				return cond.DeepCopy()
			}
		}
	}
	return nil
}
