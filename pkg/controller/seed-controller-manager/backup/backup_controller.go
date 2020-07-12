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
	"go.etcd.io/etcd/clientv3"
	"go.uber.org/zap"
	"io"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"os"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

const (
	ControllerName       = "kubermatic_backup_controller"
	defaultClusterSize   = 3
	BackupPhaseRunning   = "Running"
	BackupPhaseCompleted = "Completed"
	BackupPhaseFailed    = "Failed"
)

// Reconciler stores necessary components that are required to create etcd backups
type Reconciler struct {
	log        *zap.SugaredLogger
	workerName string
	ctrlruntimeclient.Client
	recorder record.EventRecorder
}

// Add creates a new Backup controller that is responsible for
// managing cluster etcd backups
func Add(
	mgr manager.Manager,
	log *zap.SugaredLogger,
	numWorkers int,
	workerName string,
) error {
	log = log.Named(ControllerName)
	client := mgr.GetClient()

	reconciler := &Reconciler{
		log:        log,
		Client:     client,
		workerName: workerName,
		recorder:   mgr.GetEventRecorderFor(ControllerName),
	}

	ctrlOptions := controller.Options{
		Reconciler:              reconciler,
		MaxConcurrentReconciles: numWorkers,
	}
	c, err := controller.New(ControllerName, mgr, ctrlOptions)
	if err != nil {
		return err
	}

	backupPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			new := e.ObjectNew.(*kubermaticv1.EtcdBackup)
			return new.Status.Phase != BackupPhaseCompleted && new.Status.Phase != BackupPhaseFailed
		},
	}
	return c.Watch(&source.Kind{Type: &kubermaticv1.EtcdBackup{}}, &handler.EnqueueRequestForObject{}, backupPredicate)
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

	log.Infof("Reconciling backup: meta=%v/%v spec.name=%v", backup.Namespace, backup.Name, backup.Spec.Name)

	backup.Status.Phase = BackupPhaseRunning
	// TODO update

	client, err := getEtcdClient(cluster)
	if err != nil {
		return nil, err
	}

	err = takeSnapshot(ctx, log, client, backup, cluster)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func getEtcdClient(cluster *kubermaticv1.Cluster) (*clientv3.Client, error) {
	clusterSize := cluster.Spec.EtcdClusterSize
	if clusterSize == 0 {
		clusterSize = defaultClusterSize
	}
	endpoints := []string{}
	for i := 0; i < clusterSize; i++ {
		endpoints = append(endpoints, fmt.Sprintf("etcd-%d.etcd.%s.svc.cluster.local:2380", i, cluster.Status.NamespaceName))
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

func takeSnapshot(ctx context.Context, log *zap.SugaredLogger, client *clientv3.Client, backup *kubermaticv1.EtcdBackup, cluster *kubermaticv1.Cluster) error {
	targetFile := fmt.Sprintf("/Users/oklischat/tmp/etcdbackups/backup-%s-%s", cluster.Name, backup.Name)
	partpath := targetFile + ".part"
	defer os.RemoveAll(partpath)

	var f *os.File
	f, err := os.OpenFile(partpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("could not open %s (%v)", partpath, err)
	}
	log.Info("created temporary db file", zap.String("path", partpath))

	var rd io.ReadCloser
	rd, err = client.Snapshot(ctx)
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

	if err = os.Rename(partpath, targetFile); err != nil {
		return fmt.Errorf("could not rename %s to %s (%v)", partpath, targetFile, err)
	}
	log.Info("saved", zap.String("path", targetFile))
	return nil
}

// hasChecksum returns "true" if the file size "n"
// has appended sha256 hash digest.
func hasChecksum(n int64) bool {
	// 512 is chosen because it's a minimum disk sector size
	// smaller than (and multiplies to) OS page size in most systems
	return (n % 512) == sha256.Size
}
