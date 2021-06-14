//nolint:golint
package sys11nodereadiness

import (
	"context"

	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
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
	ControllerName                                  = "sys11_node_readiness_controller"
	conditionTypeNodeReady corev1.NodeConditionType = "MetakubeNodeReady"
	// NotReadyTaintKey is a key to taint not ready nodes.
	NotReadyTaintKey string = "node.syseleven.de/not-ready"
)

var (
	taintTemplate = &corev1.Taint{
		Key:    NotReadyTaintKey,
		Effect: corev1.TaintEffectNoSchedule,
	}
)

type reconciler struct {
	log      *zap.SugaredLogger
	client   ctrlruntimeclient.Client
	recorder record.EventRecorder
}

func Add(mgr manager.Manager, log *zap.SugaredLogger) error {
	log = log.Named(ControllerName)

	reconciler := &reconciler{
		log:      log,
		client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	ctrlOptions := controller.Options{
		Reconciler: reconciler,
	}
	c, err := controller.New(ControllerName, mgr, ctrlOptions)
	if err != nil {
		return err
	}

	predicates := predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(_ event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
	return c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{}, predicates)
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := r.log.With("Node", request.Name)
	log.Debug("Reconciling")

	err := r.reconcile(ctx, log, request.Name)
	if err != nil {
		log.Errorw("Reconciling failed", zap.Error(err))
	}
	return reconcile.Result{}, err
}

func (r *reconciler) reconcile(ctx context.Context, log *zap.SugaredLogger, nodeName string) error {
	return r.updateNode(ctx, nodeName, func(node *corev1.Node) bool {
		cond := getReadinessNodeCondition(node)
		if cond == nil || cond.Status != corev1.ConditionTrue {
			// make sure the taint is present
			for _, taint := range node.Spec.Taints {
				if taint.MatchTaint(taintTemplate) {
					return false
				}
			}
			log.Debug("Adding taint to node")
			node.Spec.Taints = append(node.Spec.Taints, *taintTemplate)
			return true

		} else {
			// make sure the taint is absent
			newTaints, modified := deleteTaint(node.Spec.Taints, taintTemplate)
			if modified {
				log.Debug("Removing taint from node")
				node.Spec.Taints = newTaints
			}
			return modified
		}
	})
}

func (r *reconciler) updateNode(ctx context.Context, name string, modify func(*corev1.Node) bool) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		// Get latest version
		node := &corev1.Node{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: name}, node); err != nil {
			if kerrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		// Apply modifications
		if modify(node) {
			// If the modify function modified the resource, update it
			return r.client.Update(ctx, node)
		} else {
			return nil
		}
	})
}

// deleteTaint removes all the taints that have the same key and effect to given taintToDelete.
// copied from kubernetes/pkg/controller/controller_utils.go
func deleteTaint(taints []corev1.Taint, taintToDelete *corev1.Taint) ([]corev1.Taint, bool) {
	newTaints := []corev1.Taint{}
	deleted := false
	for i := range taints {
		if taintToDelete.MatchTaint(&taints[i]) {
			deleted = true
			continue
		}
		newTaints = append(newTaints, taints[i])
	}
	return newTaints, deleted
}

func getReadinessNodeCondition(node *corev1.Node) *corev1.NodeCondition {
	if len(node.Status.Conditions) == 0 {
		return nil
	} else {
		for _, cond := range node.Status.Conditions {
			if cond.Type == conditionTypeNodeReady {
				return &cond
			}
		}
	}
	return nil
}
