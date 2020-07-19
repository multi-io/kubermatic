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
	"crypto/sha256"
	"fmt"
	kubermaticv1 "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1"
	kubermaticv1helper "github.com/kubermatic/kubermatic/pkg/crd/kubermatic/v1/helper"
	"github.com/minio/minio-go"
	"go.etcd.io/etcd/clientv3"
	"go.uber.org/zap"
	"io"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"os"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

const (
	ControllerName     = "kubermatic_backup_controller"
	defaultClusterSize = 3
)

// Reconciler stores necessary components that are required to create etcd backups
type Reconciler struct {
	log        *zap.SugaredLogger
	workerName string
	ctrlruntimeclient.Client
	recorder          record.EventRecorder
	snapshotDir       string
	s3Endpoint        string
	s3BucketName      string
	s3AccessKeyID     string
	s3SecretAccessKey string
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
		log:               log,
		Client:            client,
		workerName:        workerName,
		recorder:          mgr.GetEventRecorderFor(ControllerName),
		snapshotDir:       snapshotDir,
		s3Endpoint:        s3Endpoint,
		s3BucketName:      s3BucketName,
		s3AccessKeyID:     s3AccessKeyID,
		s3SecretAccessKey: s3SecretAccessKey,
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
	if cluster.Status.ExtendedHealth.Apiserver != kubermaticv1.HealthStatusUp {
		log.Debug("API server is not running, trying again in 10 seconds")
		return &reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if backupCreated(backup) {
		return nil, nil
	}

	log.Debugf("Reconciling backup: meta=%v/%v spec.name=%v", backup.Namespace, backup.Name, backup.Spec.Name)

	err := r.takeSnapshot(ctx, log, backup, cluster)
	if err != nil {
		return nil, err
	}

	defer r.cleanupSnapshot(ctx, log, backup, cluster)

	err = r.uploadSnapshot(ctx, log, backup, cluster)
	if err != nil {
		return nil, err
	}

	if err = r.setAndPersistBackupCondition(ctx, backup, kubermaticv1.EtcdBackupCreated, corev1.ConditionTrue); err != nil {
		return nil, fmt.Errorf("failed to set add EtcdBackupCreated Condition: %v", err)
	}

	return nil, nil
}

func (r *Reconciler) setAndPersistBackupCondition(ctx context.Context, backup *kubermaticv1.EtcdBackup, condType kubermaticv1.EtcdBackupConditionType, status corev1.ConditionStatus) error {
	_, cond := getBackupCondition(backup, condType)
	if cond != nil && cond.Status == status {
		return nil
	}
	oldBackup := backup.DeepCopy()
	setBackupCondition(backup, condType, status)
	return r.Client.Patch(ctx, backup, ctrlruntimeclient.MergeFrom(oldBackup))
}

func backupCreated(backup *kubermaticv1.EtcdBackup) bool {
	_, cond := getBackupCondition(backup, kubermaticv1.EtcdBackupCreated)
	if cond != nil && cond.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

func setBackupCondition(backup *kubermaticv1.EtcdBackup, condType kubermaticv1.EtcdBackupConditionType, status corev1.ConditionStatus) {
	idx, cond := getBackupCondition(backup, condType)
	if cond == nil {
		cond = &kubermaticv1.EtcdBackupCondition{}
		cond.Type = condType
		cond.Status = status
		cond.LastHeartbeatTime = metav1.Now()
		cond.LastTransitionTime = metav1.Now()
		backup.Status.Conditions = append(backup.Status.Conditions, *cond)
		return
	}
	if cond.Status != status {
		cond.LastTransitionTime = metav1.Now()
		cond.Status = status
	}
	cond.LastHeartbeatTime = metav1.Now()
	backup.Status.Conditions[idx] = *cond
}

func getBackupCondition(backup *kubermaticv1.EtcdBackup, condType kubermaticv1.EtcdBackupConditionType) (int, *kubermaticv1.EtcdBackupCondition) {
	for i, c := range backup.Status.Conditions {
		if c.Type == condType {
			return i, &c
		}
	}
	return -1, nil
}

func backupFileName(backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) string {
	return fmt.Sprintf("%s-%s", cluster.Name, backup.Name)
}

func (r *Reconciler) takeSnapshot(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	client, err := getEtcdClient(cluster)
	if err != nil {
		return err
	}

	snapshotFileName := fmt.Sprintf("/%s/%s", r.snapshotDir, backupFileName(backup, cluster))
	partFile := snapshotFileName + ".part"
	defer os.RemoveAll(partFile)

	var f *os.File
	f, err = os.OpenFile(partFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("could not open %s (%v)", partFile, err)
	}
	log.Info("created temporary db file", zap.String("path", partFile))

	var rd io.ReadCloser
	deadlinedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rd, err = client.Snapshot(deadlinedCtx)
	if err != nil {
		return err
	}
	log.Info("fetching snapshot")
	var size int64
	size, err = io.Copy(f, rd)
	if err != nil {
		return err
	}
	if !hasChecksum(size) {
		return fmt.Errorf("sha256 checksum not found [bytes: %d]", size)
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	log.Info("fetched snapshot")

	if err = os.Rename(partFile, snapshotFileName); err != nil {
		return fmt.Errorf("could not rename %s to %s (%v)", partFile, snapshotFileName, err)
	}
	log.Info("saved", zap.String("path", snapshotFileName))
	return nil
}

func getEtcdClient(cluster *kubermaticv1.Cluster) (*clientv3.Client, error) {
	clusterSize := cluster.Spec.ComponentsOverride.Etcd.ClusterSize
	if clusterSize == 0 {
		clusterSize = defaultClusterSize
	}
	endpoints := []string{}
	for i := 0; i < clusterSize; i++ {
		//endpoints = append(endpoints, fmt.Sprintf("etcd-%d.etcd.%s.svc.cluster.local:2380", i, cluster.Status.NamespaceName))
		endpoints = append(endpoints, fmt.Sprintf("127.0.0.1:%v380", 2+i)) // local debugging
	}
	var err error
	for i := 0; i < 5; i++ {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: 2 * time.Second,
		})
		if err == nil && cli != nil {
			return cli, nil
		}
		time.Sleep(5 * time.Second)
	}
	return nil, fmt.Errorf("failed to establish client connection: %v", err)
}

// hasChecksum returns "true" if the file size "n"
// has appended sha256 hash digest.
func hasChecksum(n int64) bool {
	// 512 is chosen because it's a minimum disk sector size
	// smaller than (and multiplies to) OS page size in most systems
	return (n % 512) == sha256.Size
}

func (r *Reconciler) uploadSnapshot(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	// TODO long-lived client, possibly one per worker (since I think it's not thread-safe)
	client, err := minio.New(r.s3Endpoint, r.s3AccessKeyID, r.s3SecretAccessKey, true)
	if err != nil {
		return err
	}
	client.SetAppInfo("kubermatic", "v0.1")

	exists, err := client.BucketExists(r.s3BucketName)
	if err != nil {
		return err
	}
	if !exists {
		log.Debugf("Creating bucket: %v", r.s3BucketName)
		if err := client.MakeBucket(r.s3BucketName, ""); err != nil {
			return err
		}
	}

	objectName := backupFileName(backup, cluster)
	snapshotFileName := fmt.Sprintf("/%s/%s", r.snapshotDir, objectName)

	_, err = client.FPutObject(r.s3BucketName, objectName, snapshotFileName, minio.PutObjectOptions{})

	return err
}

func (r *Reconciler) cleanupSnapshot(ctx context.Context, log *zap.SugaredLogger, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	snapshotFileName := fmt.Sprintf("/%s/%s", r.snapshotDir, backupFileName(backup, cluster))
	return os.RemoveAll(snapshotFileName)
}
