package middleware

import (
	"context"
	"fmt"
	"net/http"

	kubermaticapiv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/handler/v1/common"
	"k8c.io/kubermatic/v2/pkg/keycloak"
	"k8c.io/kubermatic/v2/pkg/provider"
	contextutil "k8c.io/kubermatic/v2/pkg/util/context"
	kubermaticcontext "k8c.io/kubermatic/v2/pkg/util/context"
	k8cerrors "k8c.io/kubermatic/v2/pkg/util/errors"

	"github.com/go-kit/kit/endpoint"
)

const (
	// KeycloakFacadeContextKey key under which the current keycloak.Facade is kept in the ctx
	KeycloakFacadeContextKey contextutil.Key = "keycloak-facade"

	// MachineDeploymentRequestProviderContextKey key under which the current MachineDeploymentRequestProvider is kept in the ctx
	MachineDeploymentRequestProviderContextKey kubermaticcontext.Key = "machinedeploymentrequest-provider"
)

func PrivilegedUserGroupVerifier(userProjectMapper provider.ProjectMemberMapper, privilegedUserGroups map[string]bool) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (response interface{}, err error) {
			var projectID string
			prjIDGetter, ok := request.(common.ProjectIDGetter)
			if ok {
				projectID = prjIDGetter.GetProjectID()
			}

			user, ok := ctx.Value(UserCRContextKey).(*kubermaticapiv1.User)
			if !ok {
				return nil, k8cerrors.New(http.StatusInternalServerError, "unable to get authenticated user object")
			}

			group, err := userProjectMapper.MapUserToGroup(user.Spec.Email, projectID)
			if err != nil {
				return nil, err
			}

			// group is in format '${group}-${projectId}'
			group = group[:len(group)-len(projectID)-1]

			if trusted, ok := privilegedUserGroups[group]; ok && trusted {
				return next(context.WithValue(ctx, UserCRContextKey, user), request)
			}

			return nil, k8cerrors.New(http.StatusForbidden, "you don't have permission to access this resource")
		}
	}
}

// Keycloak is a middleware that injects the Keycloak client facade into the ctx
func Keycloak(keycloakFacade keycloak.Facade) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (response interface{}, err error) {
			ctx = context.WithValue(ctx, KeycloakFacadeContextKey, keycloakFacade)
			return next(ctx, request)
		}
	}
}

// MachineDeploymentRequests is a middleware that injects the current MachineDeploymentRequestProvider into the ctx
func MachineDeploymentRequests(mdrProviderGetter provider.MachineDeploymentRequestProviderGetter, seedsGetter provider.SeedsGetter) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (response interface{}, err error) {
			seeds, err := seedsGetter(ctx)
			if err != nil {
				return nil, err
			}
			seedCluster := request.(seedClusterGetter).GetSeedCluster()
			seed, found := seeds[seedCluster.SeedName]
			if !found {
				return nil, fmt.Errorf("couldn't find seed %q", seedCluster.SeedName)
			}

			mdrProvider, err := mdrProviderGetter(seed)
			if err != nil {
				return nil, err
			}
			ctx = context.WithValue(ctx, MachineDeploymentRequestProviderContextKey, mdrProvider)
			return next(ctx, request)
		}
	}
}
