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

package provider

import (
	"context"
	"fmt"

	kubermaticv1 "k8c.io/kubermatic/v2/pkg/crd/kubermatic/v1"
	"k8c.io/kubermatic/v2/pkg/util/restmapper"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// emptySeedMap is returned when the default seed is not present.
	emptySeedMap = map[string]*kubermaticv1.Seed{}
)

// SeedGetter is a function to retrieve a single seed
type SeedGetter = func(ctx context.Context) (*kubermaticv1.Seed, error)

// SeedsGetter is a function to retrieve a list of seeds
type SeedsGetter = func(ctx context.Context) (map[string]*kubermaticv1.Seed, error)

// SeedKubeconfigGetter is used to fetch the kubeconfig for a given seed
type SeedKubeconfigGetter = func(ctx context.Context, seed *kubermaticv1.Seed) (*rest.Config, error)

// SeedClientGetter is used to get a ctrlruntimeclient for a given seed
type SeedClientGetter = func(ctx context.Context, seed *kubermaticv1.Seed) (ctrlruntimeclient.Client, error)

// ClusterProviderGetter is used to get a clusterProvider
type ClusterProviderGetter = func(ctx context.Context, seed *kubermaticv1.Seed) (ClusterProvider, error)

// AddonProviderGetter is used to get an AddonProvider
type AddonProviderGetter = func(seed *kubermaticv1.Seed) (AddonProvider, error)

// MachineDeploymentRequestProviderGetter is used to get an MachineDeploymentRequestProvider
type MachineDeploymentRequestProviderGetter = func(seed *kubermaticv1.Seed) (MachineDeploymentRequestProvider, error)

// ClusterTemplateInstanceProviderGetter is used to get a ClusterTemplateInstanceProvider
type ClusterTemplateInstanceProviderGetter = func(seed *kubermaticv1.Seed) (ClusterTemplateInstanceProvider, error)

// EtcdBackupConfigProviderGetter is used to get a EtcdBackupConfigProvider
type EtcdBackupConfigProviderGetter = func(seed *kubermaticv1.Seed) (EtcdBackupConfigProvider, error)

// EtcdBackupConfigProjectProviderGetter is used to get a EtcdBackupConfigProjectProvider
type EtcdBackupConfigProjectProviderGetter = func(seeds map[string]*kubermaticv1.Seed) (EtcdBackupConfigProjectProvider, error)

// EtcdRestoreProviderGetter is used to get a EtcdRestoreProvider
type EtcdRestoreProviderGetter = func(seed *kubermaticv1.Seed) (EtcdRestoreProvider, error)

// EtcdRestoreProjectProviderGetter is used to get a EtcdRestoreProjectProvider
type EtcdRestoreProjectProviderGetter = func(seeds map[string]*kubermaticv1.Seed) (EtcdRestoreProjectProvider, error)

// BackupCredentialsProviderGetter is used to get a BackupCredentialsProvider
type BackupCredentialsProviderGetter = func(seed *kubermaticv1.Seed) (BackupCredentialsProvider, error)

// SeedGetterFactory returns a SeedGetter. It has validation of all its arguments
func SeedGetterFactory(client ctrlruntimeclient.Reader, seedName string, namespace string) SeedGetter {
	return func(ctx context.Context) (*kubermaticv1.Seed, error) {
		seed := &kubermaticv1.Seed{}
		if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: seedName}, seed); err != nil {
			// allow callers to handle this gracefully
			if kerrors.IsNotFound(err) {
				return nil, err
			}

			return nil, fmt.Errorf("failed to get seed %q: %v", seedName, err)
		}

		seed.SetDefaults()

		return seed, nil
	}
}

func SeedsGetterFactory(client ctrlruntimeclient.Client, namespace string) SeedsGetter {
	return func(ctx context.Context) (map[string]*kubermaticv1.Seed, error) {
		seeds := kubermaticv1.SeedList{}
		if err := client.List(ctx, &seeds, ctrlruntimeclient.InNamespace(namespace)); err != nil {
			if kerrors.IsNotFound(err) {
				// We should not fail if no seed exists and just return an
				// empty map.
				return emptySeedMap, nil
			}

			return nil, fmt.Errorf("failed to get seeds: %v", err)
		}

		result := make(map[string]*kubermaticv1.Seed)
		for i := range seeds.Items {
			seeds.Items[i].SetDefaults()
			result[seeds.Items[i].Name] = &seeds.Items[i]
		}
		return result, nil
	}
}

func SeedKubeconfigGetterFactory(client ctrlruntimeclient.Client) SeedKubeconfigGetter {
	return func(ctx context.Context, seed *kubermaticv1.Seed) (*rest.Config, error) {
		secret := &corev1.Secret{}
		name := types.NamespacedName{
			Namespace: seed.Spec.Kubeconfig.Namespace,
			Name:      seed.Spec.Kubeconfig.Name,
		}
		if name.Namespace == "" {
			name.Namespace = seed.Namespace
		}
		if err := client.Get(ctx, name, secret); err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig secret %q: %v", name.String(), err)
		}

		fieldPath := seed.Spec.Kubeconfig.FieldPath
		if len(fieldPath) == 0 {
			fieldPath = DefaultKubeconfigFieldPath
		}
		if _, exists := secret.Data[fieldPath]; !exists {
			return nil, fmt.Errorf("secret %q has no key %q", name.String(), fieldPath)
		}

		cfg, err := clientcmd.RESTConfigFromKubeConfig(secret.Data[fieldPath])
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %v", err)
		}
		return cfg, nil
	}
}

// SeedClientGetterFactory returns a SeedClientGetter. It uses a RestMapperCache to cache
// the discovery data, which considerably speeds up client creation.
func SeedClientGetterFactory(client ctrlruntimeclient.Client) SeedClientGetter {
	cache := restmapper.New()
	return func(ctx context.Context, seed *kubermaticv1.Seed) (ctrlruntimeclient.Client, error) {
		var secret corev1.Secret
		err := client.Get(ctx, types.NamespacedName{
			Namespace: seed.Spec.Kubeconfig.Namespace,
			Name:      seed.Spec.Kubeconfig.Name,
		}, &secret)
		if err != nil {
			return nil, fmt.Errorf("failed to get secret to connect to Seed: %v", err)
		}
		d := secret.Data[DefaultKubeconfigFieldPath]
		if len(d) == 0 {
			return nil, fmt.Errorf("no '%s' in kubeconfig secret", DefaultKubeconfigFieldPath)
		}
		cfg, err := clientcmd.RESTConfigFromKubeConfig(d)
		if err != nil {
			return nil, fmt.Errorf("failed to build client for seed '%s': %v", seed.Name, err)
		}
		return cache.Client(cfg)
	}
}
