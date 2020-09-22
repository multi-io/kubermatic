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
	"fmt"
	"github.com/minio/minio-go"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	kubermaticv1helper "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1/helper"
	kuberneteshelper "k8c.io/kubermatic/v2/pkg/kubernetes"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/reconciling"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"reflect"
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

	incompleteRestorePredicates := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			restore := e.Object.(*kubermaticv1.EtcdRestore)
			return restore.Status.Phase != kubermaticv1.EtcdRestorePhaseCompleted
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			restore := e.ObjectNew.(*kubermaticv1.EtcdRestore)
			return restore.Status.Phase != kubermaticv1.EtcdRestorePhaseCompleted
		},
	}

	return c.Watch(&source.Kind{Type: &kubermaticv1.EtcdRestore{}}, &handler.EnqueueRequestForObject{}, incompleteRestorePredicates)
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

	if cluster.Labels[kubermaticv1.WorkerNameLabelKey] != r.workerName {
		return reconcile.Result{}, nil
	}

	result, err := r.reconcile(ctx, log, restore, cluster)
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

	if restore.Status.Phase == kubermaticv1.EtcdRestorePhaseCompleted {
		return nil, nil
	}

	if restore.Status.Phase == kubermaticv1.EtcdRestorePhaseStsRebuilding {
		return r.rebuildEtcdStatefulset(ctx, log, restore, cluster)
	}

	secretData, err := r.ensureBackupDownloadCredentialsSecret(ctx, restore, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create s3 settings secret")
	}

	// check that the backup to restore from exists and is accessible
	s3Client, bucketName, err := r.getS3Client(secretData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read s3 settings from kube-sytem secret/configmap")
	}

	objectName := fmt.Sprintf("%s-%s", cluster.GetName(), restore.Spec.BackupName)
	if _, err := s3Client.StatObject(bucketName, objectName, minio.StatObjectOptions{}); err != nil {
		return nil, fmt.Errorf("could not access backup object %s: %w", objectName, err)
	}

	// pause cluster
	if err := r.updateCluster(ctx, cluster, func(cluster *kubermaticv1.Cluster) {
		cluster.Spec.Pause = true
	}); err != nil {
		return nil, fmt.Errorf("failed to pause cluster: %v", err)
	}

	if err := r.updateRestore(ctx, restore, func(restore *kubermaticv1.EtcdRestore) {
		restore.Status.Phase = kubermaticv1.EtcdRestorePhaseStarted
		kuberneteshelper.AddFinalizer(restore, RebuildStatefulsetFinalizer)
	}); err != nil {
		return nil, fmt.Errorf("failed to add finalizer: %v", err)
	}

	// delete etcd sts
	sts := &v1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Namespace: cluster.Status.NamespaceName, Name: resources.EtcdStatefulSetName}, sts)
	if err == nil {
		if err := r.Delete(ctx, sts); err != nil && !kerrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete etcd statefulset: %v", err)
		}
	} else if !kerrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get etcd statefulset: %v", err)
	}

	// delete PVCs
	pvcSelector, err := labels.Parse(fmt.Sprintf("%s=%s", resources.AppLabelKey, resources.EtcdStatefulSetName))
	if err != nil {
		return nil, fmt.Errorf("software bug: failed to parse etcd pvc selector: %v", err)
	}

	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, &ctrlruntimeclient.ListOptions{Namespace: cluster.Status.NamespaceName, LabelSelector: pvcSelector}); err != nil {
		return nil, fmt.Errorf("failed to list pvcs (%v): %v", pvcSelector.String(), err)
	}

	for _, pvc := range pvcs.Items {
		deletePropagationForeground := metav1.DeletePropagationForeground
		delOpts := &ctrlruntimeclient.DeleteOptions{
			PropagationPolicy: &deletePropagationForeground,
		}
		if err := r.Delete(ctx, &pvc, delOpts); err != nil {
			return nil, fmt.Errorf("failed to delete pvc %v: %v", pvc.GetName(), err)
		}
	}

	if len(pvcs.Items) > 0 {
		// some PVCs still present -- wait
		return &reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := r.updateRestore(ctx, restore, func(restore *kubermaticv1.EtcdRestore) {
		restore.Status.Phase = kubermaticv1.EtcdRestorePhaseStsRebuilding
	}); err != nil {
		return nil, fmt.Errorf("failed to proceed to sts rebuilding phase: %v", err)
	}

	return r.rebuildEtcdStatefulset(ctx, log, restore, cluster)
}

func (r *Reconciler) ensureBackupDownloadCredentialsSecret(ctx context.Context, restore *kubermaticv1.EtcdRestore, cluster *kubermaticv1.Cluster) (map[string]string, error) {
	if restore.Spec.BackupDownloadCredentialsSecret != "" {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: cluster.Status.NamespaceName, Name: restore.Spec.BackupDownloadCredentialsSecret}, secret); err != nil {
			return nil, errors.Wrapf(err, "Failed to get BackupDownloadCredentialsSecret credentials secret %v/%v", cluster.Status.NamespaceName, restore.Spec.BackupDownloadCredentialsSecret)
		}

		secretData := make(map[string]string)
		for k, v := range secret.Data {
			secretData[k] = string(v)
		}

		return secretData, nil
	}

	// create BackupDownloadCredentialsSecret containing values from kube-system/s3-credentials / kube-system/s3-settings

	credsSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: resources.EtcdRestoreS3CredentialsSecret}, credsSecret); err != nil {
		return nil, errors.Wrapf(err, "Failed to get s3 credentials secret %v/%v", metav1.NamespaceSystem, resources.EtcdRestoreS3CredentialsSecret)
	}
	settingsConfigMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceSystem, Name: resources.EtcdRestoreS3SettingsConfigMap}, settingsConfigMap); err != nil {
		return nil, errors.Wrapf(err, "Failed to get s3 settings configmap %v/%v", metav1.NamespaceSystem, resources.EtcdRestoreS3SettingsConfigMap)
	}

	secretData := make(map[string]string)
	for k, v := range credsSecret.Data {
		secretData[k] = string(v)
	}
	for k, v := range settingsConfigMap.Data {
		secretData[k] = v
	}

	creator := func(se *corev1.Secret) (*corev1.Secret, error) {
		if se.Data == nil {
			se.Data = map[string][]byte{}
		}
		for k, v := range secretData {
			se.Data[k] = []byte(v)
		}
		return se, nil
	}

	wrappedCreator := reconciling.SecretObjectWrapper(creator)
	wrappedCreator = reconciling.OwnerRefWrapper(resources.GetEtcdRestoreRef(restore))(wrappedCreator)

	secretName := fmt.Sprintf("%s-backupdownload-%s", restore.Name, rand.String(10))

	if err := reconciling.EnsureNamedObject(
		ctx,
		types.NamespacedName{Namespace: cluster.Status.NamespaceName, Name: secretName},
		wrappedCreator, r.Client, &corev1.Secret{}, false); err != nil {
		return nil, fmt.Errorf("failed to ensure Secret %s/%s: %v", cluster.Status.NamespaceName, secretName, err)
	}

	if err := r.updateRestore(ctx, restore, func(restore *kubermaticv1.EtcdRestore) {
		restore.Spec.BackupDownloadCredentialsSecret = secretName
	}); err != nil {
		return nil, fmt.Errorf("failed to write ercdrestore.backupDownloadCredentialsSecret: %v", err)
	}

	return secretData, nil
}

func (r *Reconciler) getS3Client(secretData map[string]string) (*minio.Client, string, error) {
	accessKeyId := secretData[resources.EtcdRestoreS3AccessKeyIdKey]
	secretAccessKey := secretData[resources.EtcdRestoreS3SecretKeyAccessKeyKey]
	bucketName := secretData[resources.EtcdRestoreS3BucketNameKey]
	endpoint := secretData[resources.EtcdRestoreS3EndpointKey]

	if bucketName == "" {
		return nil, "", fmt.Errorf("s3 bucket name not set")
	}
	if endpoint == "" {
		endpoint = resources.EtcdRestoreDefaultS3SEndpoint
	}

	client, err := minio.New(endpoint, accessKeyId, secretAccessKey, true)
	if err != nil {
		return nil, "", errors.Wrap(err, "error creating s3 client")
	}
	client.SetAppInfo("kubermatic", "v0.1")

	return client, bucketName, nil
}

func (r *Reconciler) rebuildEtcdStatefulset(ctx context.Context, log *zap.SugaredLogger, restore *kubermaticv1.EtcdRestore, cluster *kubermaticv1.Cluster) (*reconcile.Result, error) {
	log.Infof("rebuildEtcdStatefulset")

	if cluster.Spec.Pause {
		if err := r.updateCluster(ctx, cluster, func(cluster *kubermaticv1.Cluster) {
			kubermaticv1helper.SetClusterCondition(
				cluster,
				kubermaticv1.ClusterConditionEtcdClusterInitialized,
				corev1.ConditionFalse,
				"",
				fmt.Sprintf("Etcd Cluster is being restored from backup %v", restore.Spec.BackupName),
			)
			cluster.Spec.Pause = false
		}); err != nil {
			return nil, fmt.Errorf("failed to reset etcd initialized status and unpause cluster: %v", err)
		}
	}

	// wait until cluster controller has recreated the etcd cluster and etcd becomes healthy again
	if !cluster.Status.HasConditionValue(kubermaticv1.ClusterConditionEtcdClusterInitialized, corev1.ConditionTrue) {
		return &reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := r.updateRestore(ctx, restore, func(restore *kubermaticv1.EtcdRestore) {
		restore.Status.Phase = kubermaticv1.EtcdRestorePhaseCompleted
		kuberneteshelper.RemoveFinalizer(restore, RebuildStatefulsetFinalizer)
	}); err != nil {
		return nil, fmt.Errorf("failed to mark restore completed: %v", err)
	}

	return nil, nil
}

func (r *Reconciler) updateCluster(ctx context.Context, cluster *kubermaticv1.Cluster, modify func(*kubermaticv1.Cluster)) error {
	oldBackup := cluster.DeepCopy()
	modify(cluster)
	if reflect.DeepEqual(oldBackup, cluster) {
		return nil
	}
	return r.Client.Patch(ctx, cluster, ctrlruntimeclient.MergeFrom(oldBackup))
}

func (r *Reconciler) updateRestore(ctx context.Context, restore *kubermaticv1.EtcdRestore, modify func(*kubermaticv1.EtcdRestore)) error {
	oldBackup := restore.DeepCopy()
	modify(restore)
	if reflect.DeepEqual(oldBackup, restore) {
		return nil
	}
	return r.Client.Patch(ctx, restore, ctrlruntimeclient.MergeFrom(oldBackup))
}
