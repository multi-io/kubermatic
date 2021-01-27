package localstoragepvwatcher

import (
	"context"

	"go.uber.org/zap"

	predicateutils "k8c.io/kubermatic/v2/pkg/controller/util/predicate"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// ControllerName is the name the controller is registered by.
	ControllerName = "syseleven_localstorage_volume_watcher"

	storageLabelKey        = "node.syseleven.de/storage"
	localStorageLabelValue = "local"
)

// Reconciler reconciles localstorage persistent volumes.
type Reconciler struct {
	log *zap.SugaredLogger
	ctrlruntimeclient.Client
	recorder record.EventRecorder
}

// Add add the controller.
func Add(

	log *zap.SugaredLogger,
	mgr manager.Manager,
	numWorkers int,
) error {
	log = log.Named(ControllerName)
	reconciler := &Reconciler{
		log:      log,
		Client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:              reconciler,
		MaxConcurrentReconciles: numWorkers,
	})
	if err != nil {
		return errors.Wrap(err, "create controller")
	}
	// reconcile PVCs in ClaimLost phase only
	LocalPersistentVolumes := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			new := e.ObjectNew.(*corev1.PersistentVolume)
			return new.Labels[storageLabelKey] == localStorageLabelValue
		},
	}

	if err := c.Watch(&source.Kind{Type: &corev1.PersistentVolume{}}, &handler.EnqueueRequestForObject{},
		LocalPersistentVolumes,
		predicateutils.ByLabel(storageLabelKey, localStorageLabelValue)); err != nil {
		return errors.Wrap(err, "create watch for PersistentVolumes")
	}
	return nil
}

// Reconcile check if a node hosting a terminating local storage PV is still present
// and removes "kubernetes.io/pv-protection" finalizer of PV if it is gone.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := r.log.With("request", request)
	log.Debug("Processing")

	pv := &corev1.PersistentVolume{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: request.Name}, pv); err != nil {
		if kerrors.IsNotFound(err) {
			log.Debug("Skipping because the PV is already gone")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	result, err := r.reconcile(ctx, log, pv)
	if err != nil {
		log.Errorw("Reconciling failed", zap.Error(err))
		r.recorder.Event(pv, corev1.EventTypeWarning, "ReconcilingError", err.Error())
	}

	return result, err
}

func (r *Reconciler) reconcile(ctx context.Context, log *zap.SugaredLogger, pv *corev1.PersistentVolume) (reconcile.Result, error) {
	if pv.Status.Phase != corev1.VolumeBound || pv.DeletionTimestamp == nil {
		return reconcile.Result{}, nil
	}

	var nodeRef *v1.OwnerReference
	for _, ref := range pv.OwnerReferences {
		if ref.Kind == "Node" {
			nodeRef = &ref
			break
		}
	}

	if nodeRef == nil {
		return reconcile.Result{}, errors.New("owner reference to a node not found")
	}

	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeRef.Name}, node); err != nil && apierrors.IsNotFound(err) {
		finalivers := make([]string, 0)
		for _, v := range pv.Finalizers {
			if v != "kubernetes.io/pv-protection" {
				finalivers = append(finalivers, v)
			}
		}
		pv.Finalizers = finalivers
		if err := r.Update(ctx, pv); err != nil {
			return reconcile.Result{}, err
		}
		r.recorder.Eventf(pv, corev1.EventTypeNormal, "OwnerNodeGone", "removed 'kubernetes.io/pv-protection' finalizer due to node being gone")
	}
	return reconcile.Result{}, nil
}
