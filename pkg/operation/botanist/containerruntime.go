// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package botanist

import (
	"context"
	"fmt"

	v1alpha1constants "github.com/gardener/gardener/pkg/apis/core/v1alpha1/constants"
	gardencorev1alpha1helper "github.com/gardener/gardener/pkg/apis/core/v1alpha1/helper"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/operation/shoot"
	"github.com/gardener/gardener/pkg/utils/flow"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	"github.com/gardener/gardener/pkg/utils/retry"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeployContainerRuntimeResources creates the `Container runtime` resource in the shoot namespace in the seed
// cluster. Gardener waits until an external controller did reconcile the resources successfully.
func (b *Botanist) DeployContainerRuntimeResources(ctx context.Context) error {
	fns := make([]flow.TaskFn, 0, len(b.Shoot.ContainerRuntimesMap))
	for _, containerRuntime := range b.Shoot.ContainerRuntimesMap {
		var (
			containerRuntimeType = containerRuntime.Spec.Type
			toApply              = extensionsv1alpha1.ContainerRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      containerRuntime.Name,
					Namespace: containerRuntime.Namespace,
				},
			}
		)

		fns = append(fns, func(ctx context.Context) error {
			return kutil.CreateOrUpdate(ctx, b.K8sSeedClient.Client(), &toApply, func() error {
				metav1.SetMetaDataAnnotation(&toApply.ObjectMeta, v1alpha1constants.GardenerOperation, v1alpha1constants.GardenerOperationReconcile)
				toApply.Spec.Type = containerRuntimeType
				return nil
			})
		})
	}

	return flow.Parallel(fns...)(ctx)
}

// DeleteStaleContainerRuntimeResources deletes unused container runtime resources from the shoot namespace in the seed.
func (b *Botanist) DeleteStaleContainerRuntimeResources(ctx context.Context) error {
	wantedContainerRuntimes := sets.NewString()
	for _, containerRuntime := range b.Shoot.ContainerRuntimesMap {
		wantedContainerRuntimes.Insert(containerRuntime.Spec.Type)
	}

	deployedContainerRuntimes := &extensionsv1alpha1.ContainerRuntimeList{}
	if err := b.K8sSeedClient.Client().List(ctx, deployedContainerRuntimes, client.InNamespace(b.Shoot.SeedNamespace)); err != nil {
		return err
	}

	fns := make([]flow.TaskFn, 0, meta.LenList(deployedContainerRuntimes))
	for _, deployedContainerRuntime := range deployedContainerRuntimes.Items {
		if !wantedContainerRuntimes.Has(deployedContainerRuntime.Spec.Type) {
			toDelete := &extensionsv1alpha1.ContainerRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployedContainerRuntime.Name,
					Namespace: deployedContainerRuntime.Namespace,
				},
			}
			fns = append(fns, func(ctx context.Context) error {
				return client.IgnoreNotFound(b.K8sSeedClient.Client().Delete(ctx, toDelete, kubernetes.DefaultDeleteOptions...))
			})
		}
	}

	return flow.Parallel(fns...)(ctx)
}

// WaitUntilContainerRuntimeResourcesReady waits until all container runtime resources report `Succeeded` in their last operation state.
// The state must be reported before the passed context is cancelled or a container runtime's timeout has been reached.
// As soon as one timeout has been overstepped the function returns an error, further waits on container runtime will be aborted.
func (b *Botanist) WaitUntilContainerRuntimeResourcesReady(ctx context.Context) error {
	fns := make([]flow.TaskFn, 0, len(b.Shoot.ContainerRuntimesMap))
	for _, containerRuntime := range b.Shoot.ContainerRuntimesMap {
		var (
			name      = containerRuntime.Name
			namespace = containerRuntime.Namespace
		)
		fns = append(fns, func(ctx context.Context) error {
			if err := retry.UntilTimeout(ctx, DefaultInterval, shoot.ExtensionDefaultTimeout, func(ctx context.Context) (bool, error) {
				req := &extensionsv1alpha1.ContainerRuntime{}
				if err := b.K8sSeedClient.Client().Get(ctx, kutil.Key(namespace, name), req); err != nil {
					return retry.SevereError(err)
				}

				if err := health.CheckExtensionObject(req); err != nil {
					b.Logger.WithError(err).Errorf("Container runtime %s/%s did not get ready yet", namespace, name)
					return retry.MinorError(err)
				}

				return retry.Ok()
			}); err != nil {
				return gardencorev1alpha1helper.DetermineError(fmt.Sprintf("failed waiting for container runtime %s to be ready: %v", name, err))
			}
			return nil
		})
	}

	return flow.ParallelExitOnError(fns...)(ctx)
}

// DeleteContainerRuntimeResources deletes all container runtime resources from the Shoot namespace in the Seed.
func (b *Botanist) DeleteContainerRuntimeResources(ctx context.Context) error {
	return b.K8sSeedClient.Client().DeleteAllOf(ctx, &extensionsv1alpha1.ContainerRuntime{}, client.InNamespace(b.Shoot.SeedNamespace))
}

// WaitUntilContainerRuntimeResourcesDeleted waits until all container runtime resources are gone or the context is cancelled.
func (b *Botanist) WaitUntilContainerRuntimeResourcesDeleted(ctx context.Context) error {
	var (
		lastError         *gardencorev1beta1.LastError
		containerRuntimes = &extensionsv1alpha1.ContainerRuntimeList{}
	)

	if err := b.K8sSeedClient.Client().List(ctx, containerRuntimes, client.InNamespace(b.Shoot.SeedNamespace)); err != nil {
		return err
	}

	fns := make([]flow.TaskFn, 0, len(containerRuntimes.Items))
	for _, containerRuntime := range containerRuntimes.Items {
		if containerRuntime.GetDeletionTimestamp() == nil {
			continue
		}

		var (
			name      = containerRuntime.Name
			namespace = containerRuntime.Namespace
			status    = containerRuntime.Status
		)

		fns = append(fns, func(ctx context.Context) error {
			if err := retry.UntilTimeout(ctx, DefaultInterval, shoot.ExtensionDefaultTimeout, func(ctx context.Context) (bool, error) {
				if err := b.K8sSeedClient.Client().Get(ctx, kutil.Key(namespace, name), &extensionsv1alpha1.ContainerRuntime{}); err != nil {
					if apierrors.IsNotFound(err) {
						return retry.Ok()
					}
					return retry.SevereError(err)
				}

				if lastErr := status.LastError; lastErr != nil {
					b.Logger.Errorf("Container runtime %s did not get deleted yet, lastError is: %s", name, lastErr.Description)
					lastError = lastErr
				}

				return retry.MinorError(gardencorev1beta1helper.WrapWithLastError(fmt.Errorf("container runtime %s is still present", name), lastError))
			}); err != nil {
				message := fmt.Sprintf("Failed waiting for container runtime delete")
				if lastError != nil {
					return gardencorev1alpha1helper.DetermineError(fmt.Sprintf("%s: %s", message, lastError.Description))
				}
				return gardencorev1alpha1helper.DetermineError(fmt.Sprintf("%s: %s", message, err.Error()))
			}
			return nil
		})
	}

	return flow.Parallel(fns...)(ctx)
}
