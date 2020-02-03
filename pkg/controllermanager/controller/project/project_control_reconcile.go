// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package project

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/operation/common"
	"github.com/gardener/gardener/pkg/utils"
	kutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	retryutils "github.com/gardener/gardener/pkg/utils/retry"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *defaultControl) reconcile(project *gardencorev1beta1.Project, projectLogger logrus.FieldLogger) error {
	var (
		ctx        = context.TODO()
		generation = project.Generation
		err        error
	)

	// Ensure that we really get the latest version of the project to prevent working with an outdated version that has
	// an unset .spec.namespace field (which would result in trying to create another namespace again).
	project, err = c.k8sGardenClient.GardenCore().CoreV1beta1().Projects().Get(project.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// If the project has no phase yet then we update it to be 'pending'.
	if len(project.Status.Phase) == 0 {
		if _, err := c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectPending)); err != nil {
			return err
		}
	}

	// We reconcile the namespace for the project: If the .spec.namespace is set then we try to claim it, if it is not
	// set then we create a new namespace with a random hash value.
	namespace, err := c.reconcileNamespaceForProject(project)
	if err != nil {
		c.recorder.Eventf(project, corev1.EventTypeWarning, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, err.Error())
		c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectFailed))
		return err
	}
	c.reportEvent(project, false, gardencorev1beta1.ProjectEventNamespaceReconcileSuccessful, "Successfully reconciled namespace %q for project %q", namespace.Name, project.Name)

	// Update the name of the created namespace in the projects '.spec.namespace' field.
	if ns := project.Spec.Namespace; ns == nil {
		project, err = kutils.TryUpdateProject(c.k8sGardenClient.GardenCore(), retry.DefaultBackoff, project.ObjectMeta, func(project *gardencorev1beta1.Project) (*gardencorev1beta1.Project, error) {
			project.Spec.Namespace = &namespace.Name
			return project, nil
		})
		if err != nil {
			c.reportEvent(project, false, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, err.Error())
			c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectFailed))

			// If we failed to update the namespace in the project specification we should try to delete
			// our created namespace again to prevent an inconsistent state.
			if err := retryutils.UntilTimeout(ctx, time.Second, time.Minute, func(context.Context) (done bool, err error) {
				if err := c.k8sGardenClient.Client().Delete(ctx, namespace, kubernetes.DefaultDeleteOptions...); err != nil {
					if apierrors.IsNotFound(err) {
						return retryutils.Ok()
					}
					return retryutils.SevereError(err)
				}

				return retryutils.MinorError(fmt.Errorf("namespace %q still exists", namespace.Name))
			}); err != nil {
				c.reportEvent(project, true, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, "Failed to delete created namespace for project %q: %v", namespace.Name, err)
			}

			return err
		}
	}

	chartRenderer, err := chartrenderer.NewForConfig(c.k8sGardenClient.RESTConfig())
	if err != nil {
		c.reportEvent(project, true, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, err.Error())
		c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectFailed))
		return err
	}
	applier, err := kubernetes.NewApplierForConfig(c.k8sGardenClient.RESTConfig())
	if err != nil {
		c.reportEvent(project, true, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, err.Error())
		c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectFailed))
		return err
	}
	chartApplier := kubernetes.NewChartApplier(chartRenderer, applier)

	// Create RBAC rules to allow project owner and project members to read, update, and delete the project.
	// We also create a RoleBinding in the namespace that binds all members to the gardener.cloud:system:project-member
	// role to ensure access for listing shoots, creating secrets, etc.
	var (
		admins  []rbacv1.Subject
		viewers []rbacv1.Subject
	)

	for _, member := range project.Spec.Members {
		if member.Role == gardencorev1beta1.ProjectMemberAdmin {
			admins = append(admins, member.Subject)
		}
		if member.Role == gardencorev1beta1.ProjectMemberViewer {
			viewers = append(viewers, member.Subject)
		}
	}

	if err := chartApplier.ApplyChart(ctx, filepath.Join(common.ChartPath, "garden-project", "charts", "project-rbac"), namespace.Name, "project-rbac", map[string]interface{}{
		"project": map[string]interface{}{
			"name":    project.Name,
			"uid":     project.UID,
			"owner":   project.Spec.Owner,
			"members": admins,
			"viewers": viewers,
		},
	}, nil); err != nil {
		c.reportEvent(project, true, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, "Error while creating RBAC rules for namespace %q: %+v", namespace.Name, err)
		c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectFailed))
		return err
	}

	// Delete legacy resources
	// TODO: This can be removed in a future version of Gardener (post v1.0 release).
	for _, obj := range []runtime.Object{
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "garden.sapcloud.io:system:project-member", Namespace: namespace.Name}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "garden.sapcloud.io:system:project-viewer", Namespace: namespace.Name}},
	} {
		if err := c.k8sGardenClient.Client().Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
			c.reportEvent(project, true, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, "Error while cleaning up legacy RBAC rules for namespace %q: %+v", namespace.Name, err)
			c.updateProjectStatus(project.ObjectMeta, setProjectPhase(gardencorev1beta1.ProjectFailed))
			return err
		}
	}

	// Update the project status to mark it as 'ready'.
	if _, err := c.updateProjectStatus(project.ObjectMeta, func(project *gardencorev1beta1.Project) (*gardencorev1beta1.Project, error) {
		project.Status.Phase = gardencorev1beta1.ProjectReady
		project.Status.ObservedGeneration = generation
		return project, nil
	}); err != nil {
		c.reportEvent(project, true, gardencorev1beta1.ProjectEventNamespaceReconcileFailed, "Error while trying to mark project as ready: %+v", err)
		return err
	}

	return nil
}

func (c *defaultControl) reconcileNamespaceForProject(project *gardencorev1beta1.Project) (*corev1.Namespace, error) {
	var (
		namespaceName = project.Spec.Namespace

		projectLabels      = namespaceLabelsFromProject(project)
		projectAnnotations = namespaceAnnotationsFromProject(project)
		ownerReference     = metav1.NewControllerRef(project, gardencorev1beta1.SchemeGroupVersion.WithKind("Project"))
	)

	if namespaceName == nil {
		obj := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName:    fmt.Sprintf("%s%s-", common.ProjectPrefix, project.Name),
				OwnerReferences: []metav1.OwnerReference{*ownerReference},
				Labels:          projectLabels,
				Annotations:     projectAnnotations,
			},
		}
		err := c.k8sGardenClient.Client().Create(context.TODO(), obj)
		return obj, err
	}

	namespace, err := kutils.TryUpdateNamespace(c.k8sGardenClient.Kubernetes(), retry.DefaultBackoff, metav1.ObjectMeta{Name: *namespaceName}, func(ns *corev1.Namespace) (*corev1.Namespace, error) {
		projectLabelsDeprecated := namespaceLabelsFromProjectDeprecated(project)
		if !apiequality.Semantic.DeepDerivative(projectLabelsDeprecated, ns.Labels) {
			return nil, fmt.Errorf("namespace cannot be used as it needs the project labels %#v", projectLabelsDeprecated)
		}

		projectAnnotationsDeprecated := namespaceAnnotationsFromProjectDeprecated(project)
		if metav1.HasAnnotation(ns.ObjectMeta, common.NamespaceProjectDeprecated) && !apiequality.Semantic.DeepDerivative(projectAnnotationsDeprecated, ns.Annotations) {
			return nil, fmt.Errorf("namespace is already in-use by another project")
		}

		ns.OwnerReferences = common.MergeOwnerReferences(ns.OwnerReferences, *ownerReference)
		ns.Labels = utils.MergeStringMaps(ns.Labels, projectLabels)
		ns.Annotations = utils.MergeStringMaps(ns.Annotations, projectAnnotations)

		return ns, nil
	})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		obj := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:            *namespaceName,
				OwnerReferences: []metav1.OwnerReference{*ownerReference},
				Labels:          projectLabels,
				Annotations:     projectAnnotations,
			},
		}
		err := c.k8sGardenClient.Client().Create(context.TODO(), obj)
		return obj, err
	}

	return namespace, nil
}
