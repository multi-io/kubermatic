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

package etcdrestore

import (
	"context"
	"go.uber.org/zap"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	kubermaticv1helper "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1/helper"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	ControllerName = "kubermatic_etcd_restore_controller"

	// RebuildStatefulsetFinalizer indicates that the restore is rebuilding the etcd statefulset
	RebuildStatefulsetFinalizer = "kubermatic.io/rebuild-statefulset"
)

// Reconciler stores necessary components that are required to restore etcd backups
type Reconciler struct {
	log        *zap.SugaredLogger
	workerName string
	ctrlruntimeclient.Client
	recorder record.EventRecorder
}

// Add creates a new etcd restore controller that is responsible for
// managing cluster etcd restores
func Add(
	mgr manager.Manager,
	log *zap.SugaredLogger,
	numWorkers int,
	workerName string,
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
	}

	ctrlOptions := controller.Options{
		Reconciler:              reconciler,
		MaxConcurrentReconciles: numWorkers,
	}
	c, err := controller.New(ControllerName, mgr, ctrlOptions)
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &kubermaticv1.EtcdRestore{}}, &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := r.log.With("request", request)
	log.Debug("Processing")

	restore := &kubermaticv1.EtcdRestore{}
	if err := r.Get(ctx, request.NamespacedName, restore); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	cluster := &kubermaticv1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.Cluster.Name}, cluster); err != nil {
		return reconcile.Result{}, err
	}

	log = r.log.With("cluster", cluster.Name, "restore", restore.Name)

	// Add a wrapping here so we can emit an event on error
	result, err := kubermaticv1helper.ClusterReconcileWrapper(
		ctx,
		r.Client,
		r.workerName,
		cluster,
		kubermaticv1.ClusterConditionEtcdRestoreControllerReconcilingSuccess,
		func() (*reconcile.Result, error) {
			return r.reconcile(ctx, log, restore, cluster)
		},
	)
	if err != nil {
		log.Errorw("Reconciling failed", zap.Error(err))
		r.recorder.Event(restore, corev1.EventTypeWarning, "ReconcilingError", err.Error())
		r.recorder.Eventf(cluster, corev1.EventTypeWarning, "ReconcilingError",
			"failed to reconcile etcd restore %q: %v", restore.Name, err)
	}
	if result == nil {
		result = &reconcile.Result{}
	}
	return *result, err
}

func (r *Reconciler) reconcile(ctx context.Context, log *zap.SugaredLogger, restore *kubermaticv1.EtcdRestore, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	log.Infof("performing etcd restore from backup %v", restore.Spec.BackupName)
	return nil, nil
}
