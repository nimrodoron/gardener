// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package openidconnectpreset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gardener/gardener/pkg/apis/core"
	settingsv1alpha1 "github.com/gardener/gardener/pkg/apis/settings/v1alpha1"
	admissioninitializer "github.com/gardener/gardener/pkg/apiserver/admission/initializer"
	settingsinformer "github.com/gardener/gardener/pkg/client/settings/informers/externalversions"
	settingslister "github.com/gardener/gardener/pkg/client/settings/listers/settings/v1alpha1"
	applier "github.com/gardener/gardener/plugin/pkg/shoot/oidc"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/admission"
)

const (
	// PluginName is the name of this admission plugin.
	PluginName = "OpenIDConnectPreset"
)

// Register registers a plugin.
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return New()
	})
}

// OpenIDConnectPreset contains listers and and admission handler.
type OpenIDConnectPreset struct {
	*admission.Handler

	oidcLister settingslister.OpenIDConnectPresetLister
	readyFunc  admission.ReadyFunc
}

var (
	_ = admissioninitializer.WantsSettingsInformerFactory(&OpenIDConnectPreset{})

	readyFuncs = []admission.ReadyFunc{}
)

// New creates a new OpenIDConnectPreset admission plugin.
func New() (*OpenIDConnectPreset, error) {
	return &OpenIDConnectPreset{
		Handler: admission.NewHandler(admission.Create),
	}, nil
}

// AssignReadyFunc assigns the ready function to the admission handler.
func (o *OpenIDConnectPreset) AssignReadyFunc(f admission.ReadyFunc) {
	o.readyFunc = f
	o.SetReadyFunc(f)
}

// SetSettingsInformerFactory gets Lister from SharedInformerFactory.
func (o *OpenIDConnectPreset) SetSettingsInformerFactory(f settingsinformer.SharedInformerFactory) {
	oidc := f.Settings().V1alpha1().OpenIDConnectPresets()
	o.oidcLister = oidc.Lister()

	readyFuncs = append(readyFuncs, oidc.Informer().HasSynced)
}

// ValidateInitialization checks whether the plugin was correctly initialized.
func (o *OpenIDConnectPreset) ValidateInitialization() error {
	if o.oidcLister == nil {
		return errors.New("missing oidcpreset lister")
	}
	return nil
}

var _ admission.MutationInterface = &OpenIDConnectPreset{}

// Admit tries to determine a OpenIDConnectPreset hosted zone for the Shoot's external domain.
func (o *OpenIDConnectPreset) Admit(ctx context.Context, a admission.Attributes, out admission.ObjectInterfaces) error {
	// Wait until the caches have been synced
	if o.readyFunc == nil {
		o.AssignReadyFunc(func() bool {
			for _, readyFunc := range readyFuncs {
				if !readyFunc() {
					return false
				}
			}
			return true
		})
	}
	if !o.WaitForReady() {
		return admission.NewForbidden(a, errors.New("not yet ready to handle request"))
	}

	// Ignore all kinds other than Shoot
	// Ignore all subresource calls
	// Ignore all operations other than CREATE
	if len(a.GetSubresource()) != 0 || a.GetKind().GroupKind() != core.Kind("Shoot") || a.GetOperation() != admission.Create {
		return nil
	}
	shoot, ok := a.GetObject().(*core.Shoot)
	if !ok {
		return apierrors.NewBadRequest("could not convert resource into Shoot object")
	}

	// Ignore if the Shoot manifest specifies OIDCConfig.
	if shoot.Spec.Kubernetes.KubeAPIServer != nil && shoot.Spec.Kubernetes.KubeAPIServer.OIDCConfig != nil {
		return nil
	}

	oidcs, err := o.oidcLister.OpenIDConnectPresets(shoot.Namespace).List(labels.Everything())
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("could not list existing openidconnectpresets: %v", err))
	}

	preset, err := filterOIDCs(oidcs, shoot)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	// We have an OpenIDConnectPreset, use it.
	if preset != nil {
		applier.ApplyOIDCConfiguration(shoot, preset)
		return nil
	}

	return nil
}

func filterOIDCs(oidcs []*settingsv1alpha1.OpenIDConnectPreset, shoot *core.Shoot) (*settingsv1alpha1.OpenIDConnectPresetSpec, error) {
	var matchedPreset *settingsv1alpha1.OpenIDConnectPreset

	for _, oidc := range oidcs {
		spec := oidc.Spec
		selector, err := metav1.LabelSelectorAsSelector(spec.ShootSelector)
		if err != nil {
			return nil, fmt.Errorf("label selector conversion failed: %v for shootSelector: %v", *spec.ShootSelector, err)
		}

		// check if the Shoot labels match the selector
		if !selector.Matches(labels.Set(shoot.Labels)) {
			continue
		}

		if matchedPreset == nil {
			matchedPreset = oidc
		} else if spec.Weight >= matchedPreset.Spec.Weight {
			if spec.Weight > matchedPreset.Spec.Weight {
				matchedPreset = oidc
			} else if strings.Compare(oidc.ObjectMeta.Name, matchedPreset.ObjectMeta.Name) > 0 {
				matchedPreset = oidc
			}
		}

	}

	if matchedPreset == nil {
		return nil, nil
	}
	return &matchedPreset.Spec, nil
}
