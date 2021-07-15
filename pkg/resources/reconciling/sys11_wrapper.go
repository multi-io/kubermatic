package reconciling

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SetRunningReplicasNumberIfExistsWrapper creates object modifier that sets replicas
// number to match what is configured in a deployment if it exists.
func SetRunningReplicasNumberIfExistsWrapper(ctx context.Context, client ctrlruntimeclient.Client) ObjectModifier {
	return func(create ObjectCreator) ObjectCreator {
		return func(existing ctrlruntimeclient.Object) (ctrlruntimeclient.Object, error) {
			obj, err := create(existing)
			if err != nil {
				return obj, err
			}
			switch o := obj.(type) {
			case *appsv1.Deployment:
				deployed := &appsv1.Deployment{}
				key := ctrlruntimeclient.ObjectKey{
					Namespace: o.Namespace,
					Name:      o.Name,
				}
				if err := client.Get(ctx, key, deployed); err != nil {
					if kubeerrors.IsNotFound(err) {
						return obj, nil
					}
					return nil, fmt.Errorf("failed to get Deployment %+v: %v", key, err)
				}
				if v := deployed.Spec.Replicas; v != nil {
					o.Spec.Replicas = v
				}
				return o, nil
			default:
				return o, fmt.Errorf(`type %q is not supported by SetRunningReplicasNumberIfExistsModifier`, o.GetObjectKind().GroupVersionKind())
			}
		}
	}
}
