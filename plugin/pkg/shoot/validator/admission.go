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

package validator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/helper"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	admissioninitializer "github.com/gardener/gardener/pkg/apiserver/admission/initializer"
	coreinformers "github.com/gardener/gardener/pkg/client/core/informers/internalversion"
	corelisters "github.com/gardener/gardener/pkg/client/core/listers/core/internalversion"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/operation/common"
	cidrvalidation "github.com/gardener/gardener/pkg/utils/validation/cidr"
	admissionutils "github.com/gardener/gardener/plugin/pkg/utils"

	"github.com/Masterminds/semver"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
)

const (
	// PluginName is the name of this admission plugin.
	PluginName = "ShootValidator"
)

// Register registers a plugin.
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return New()
	})
}

// ValidateShoot contains listers and and admission handler.
type ValidateShoot struct {
	*admission.Handler
	cloudProfileLister corelisters.CloudProfileLister
	seedLister         corelisters.SeedLister
	shootLister        corelisters.ShootLister
	projectLister      corelisters.ProjectLister
	readyFunc          admission.ReadyFunc
}

var (
	_ = admissioninitializer.WantsInternalCoreInformerFactory(&ValidateShoot{})

	readyFuncs = []admission.ReadyFunc{}
)

// New creates a new ValidateShoot admission plugin.
func New() (*ValidateShoot, error) {
	return &ValidateShoot{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}, nil
}

// AssignReadyFunc assigns the ready function to the admission handler.
func (v *ValidateShoot) AssignReadyFunc(f admission.ReadyFunc) {
	v.readyFunc = f
	v.SetReadyFunc(f)
}

// SetInternalCoreInformerFactory gets Lister from SharedInformerFactory.
func (v *ValidateShoot) SetInternalCoreInformerFactory(f coreinformers.SharedInformerFactory) {
	seedInformer := f.Core().InternalVersion().Seeds()
	v.seedLister = seedInformer.Lister()

	shootInformer := f.Core().InternalVersion().Shoots()
	v.shootLister = shootInformer.Lister()

	cloudProfileInformer := f.Core().InternalVersion().CloudProfiles()
	v.cloudProfileLister = cloudProfileInformer.Lister()

	projectInformer := f.Core().InternalVersion().Projects()
	v.projectLister = projectInformer.Lister()

	readyFuncs = append(readyFuncs, seedInformer.Informer().HasSynced, shootInformer.Informer().HasSynced, cloudProfileInformer.Informer().HasSynced, projectInformer.Informer().HasSynced)
}

// ValidateInitialization checks whether the plugin was correctly initialized.
func (v *ValidateShoot) ValidateInitialization() error {
	if v.cloudProfileLister == nil {
		return errors.New("missing cloudProfile lister")
	}
	if v.seedLister == nil {
		return errors.New("missing seed lister")
	}
	if v.shootLister == nil {
		return errors.New("missing shoot lister")
	}
	if v.projectLister == nil {
		return errors.New("missing project lister")
	}
	return nil
}

var _ admission.MutationInterface = &ValidateShoot{}

// Admit validates the Shoot details against the referenced CloudProfile.
func (v *ValidateShoot) Admit(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	// Wait until the caches have been synced
	if v.readyFunc == nil {
		v.AssignReadyFunc(func() bool {
			for _, readyFunc := range readyFuncs {
				if !readyFunc() {
					return false
				}
			}
			return true
		})
	}
	if !v.WaitForReady() {
		return admission.NewForbidden(a, errors.New("not yet ready to handle request"))
	}

	// Ignore all kinds other than Shoot
	if a.GetKind().GroupKind() != core.Kind("Shoot") {
		return nil
	}

	// Ignore updates to shoot status or other subresources
	if a.GetSubresource() != "" {
		return nil
	}

	// Ignore updates if shoot spec hasn't changed
	if a.GetOperation() == admission.Update {
		newShoot, ok := a.GetObject().(*core.Shoot)
		if !ok {
			return apierrors.NewInternalError(errors.New("could not convert resource into Shoot object"))
		}
		oldShoot, ok := a.GetOldObject().(*core.Shoot)
		if !ok {
			return apierrors.NewInternalError(errors.New("could not convert old resource into Shoot object"))
		}

		// do not ignore metadata updates to detect and prevent removal of the gardener finalizer or unwanted changes to annotations
		if reflect.DeepEqual(newShoot.Spec, oldShoot.Spec) && reflect.DeepEqual(newShoot.ObjectMeta, oldShoot.ObjectMeta) {
			return nil
		}

		if newShoot.Spec.Provider.Type != oldShoot.Spec.Provider.Type {
			return apierrors.NewBadRequest("shoot provider type was changed which is not allowed")
		}
	}

	shoot, ok := a.GetObject().(*core.Shoot)
	if !ok {
		return apierrors.NewInternalError(errors.New("could not convert resource into Shoot object"))
	}

	cloudProfile, err := v.cloudProfileLister.Get(shoot.Spec.CloudProfileName)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("could not find referenced cloud profile: %+v", err.Error()))
	}

	var seed *core.Seed
	if shoot.Spec.SeedName != nil {
		seed, err = v.seedLister.Get(*shoot.Spec.SeedName)
		if err != nil {
			return apierrors.NewBadRequest(fmt.Sprintf("could not find referenced seed: %+v", err.Error()))
		}
	}

	project, err := admissionutils.GetProject(shoot.Namespace, v.projectLister)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("could not find referenced project: %+v", err.Error()))
	}

	switch a.GetOperation() {
	case admission.Create:
		// We currently use the identifier "shoot-<project-name>-<shoot-name> in nearly all places for old Shoots, but have
		// changed that to "shoot--<project-name>-<shoot-name>": when creating infrastructure resources, Kubernetes resources,
		// DNS names, etc., then this identifier is used to tag/name the resources. Some of those resources have length
		// constraints that this identifier must not exceed 30 characters, thus we need to check whether Shoots do not exceed
		// this limit. The project name is a label on the namespace. If it is not found, the namespace name itself is used as
		// project name. These checks should only be performed for CREATE operations (we do not want to reject changes to existing
		// Shoots in case the limits are changed in the future).
		var lengthLimit = 21
		if len(project.Name+shoot.Name) > lengthLimit {
			return apierrors.NewBadRequest(fmt.Sprintf("the length of the shoot name and the project name must not exceed %d characters (project: %s; shoot: %s)", lengthLimit, project.Name, shoot.Name))
		}
		if strings.Contains(project.Name, "--") {
			return apierrors.NewBadRequest(fmt.Sprintf("the project name must not contain two consecutive hyphens (project: %s)", project.Name))
		}
		// We don't want new Shoots to be created in Projects which were already marked for deletion.
		if project.DeletionTimestamp != nil {
			return admission.NewForbidden(a, fmt.Errorf("cannot create shoot '%s' in project '%s' already marked for deletion", shoot.Name, project.Name))
		}
	}

	changed, err := seedChanged(a)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	needCheckForProtectedSeed := changed || a.GetOperation() == admission.Create
	// Check whether seed is protected or not only if the shoot.spec.seedName has been updated. In case it is protected then we only allow Shoot resources to reference it which are part of the Garden namespace.
	if needCheckForProtectedSeed && shoot.Namespace != v1beta1constants.GardenNamespace && seed != nil && helper.TaintsHave(seed.Spec.Taints, core.SeedTaintProtected) {
		return admission.NewForbidden(a, fmt.Errorf("forbidden to use a protected seed"))
	}

	// We don't allow shoot to be created on the seed which is already marked to be deleted.
	if seed != nil && seed.DeletionTimestamp != nil && a.GetOperation() == admission.Create {
		return admission.NewForbidden(a, fmt.Errorf("cannot create shoot '%s' on seed '%s' already marked for deletion", shoot.Name, seed.Name))
	}

	if shoot.Spec.Provider.Type != cloudProfile.Spec.Type {
		return apierrors.NewBadRequest(fmt.Sprintf("cloud provider in shoot (%s) is not equal to cloud provider in profile (%s)", shoot.Spec.Provider.Type, cloudProfile.Spec.Type))
	}

	// We only want to validate fields in the Shoot against the CloudProfile/Seed constraints which have changed.
	// On CREATE operations we just use an empty Shoot object, forcing the validator functions to always validate.
	// On UPDATE operations we fetch the current Shoot object.
	var oldShoot = &core.Shoot{}
	if a.GetOperation() == admission.Update {
		old, ok := a.GetOldObject().(*core.Shoot)
		if !ok {
			return apierrors.NewInternalError(errors.New("could not convert old resource into Shoot object"))
		}
		oldShoot = old
	}

	var (
		validationContext = &validationContext{
			cloudProfile: cloudProfile,
			seed:         seed,
			shoot:        shoot,
			oldShoot:     oldShoot,
		}
		allErrs field.ErrorList
	)

	if seed != nil && seed.DeletionTimestamp != nil {
		newMeta := shoot.ObjectMeta
		oldMeta := *oldShoot.ObjectMeta.DeepCopy()

		// disallow any changes to the annotations of a shoot that references a seed which is already marked for deletion
		// except changes to the deletion confirmation annotation
		if !reflect.DeepEqual(newMeta.Annotations, oldMeta.Annotations) {
			confimationAnnotations := []string{common.ConfirmationDeletion, common.ConfirmationDeletionDeprecated}
			for _, annotation := range confimationAnnotations {
				newConfirmation, newHasConfirmation := newMeta.Annotations[annotation]

				// copy the new confirmation value to the old annotations to see if
				// anything else was changed other than the confirmation annotation
				if newHasConfirmation {
					if oldMeta.Annotations == nil {
						oldMeta.Annotations = make(map[string]string)
					}
					oldMeta.Annotations[annotation] = newConfirmation
				}
			}

			if !reflect.DeepEqual(newMeta.Annotations, oldMeta.Annotations) {
				return admission.NewForbidden(a, fmt.Errorf("cannot update annotations of shoot '%s' on seed '%s' already marked for deletion: only the '%s' annotation can be changed", shoot.Name, seed.Name, common.ConfirmationDeletion))
			}
		}

		if !reflect.DeepEqual(shoot.Spec, oldShoot.Spec) {
			return admission.NewForbidden(a, fmt.Errorf("cannot update spec of shoot '%s' on seed '%s' already marked for deletion", shoot.Name, seed.Name))
		}
	}

	// Allow removal of `gardener` finalizer only if the Shoot deletion has completed successfully
	if len(shoot.Status.TechnicalID) > 0 && shoot.Status.LastOperation != nil {
		oldFinalizers := sets.NewString(oldShoot.Finalizers...)
		newFinalizers := sets.NewString(shoot.Finalizers...)

		if oldFinalizers.Has(core.GardenerName) && !newFinalizers.Has(core.GardenerName) {
			lastOperation := shoot.Status.LastOperation
			deletionSucceeded := lastOperation.Type == core.LastOperationTypeDelete && lastOperation.State == core.LastOperationStateSucceeded && lastOperation.Progress == 100

			if !deletionSucceeded {
				return admission.NewForbidden(a, fmt.Errorf("finalizer %q cannot be removed because shoot deletion has not completed successfully yet", core.GardenerName))
			}
		}
	}

	// Prevent Shoots from getting hibernated in case they have problematic webhooks.
	// Otherwise, we can never wake up this shoot cluster again.
	oldIsHibernated := oldShoot.Spec.Hibernation != nil && oldShoot.Spec.Hibernation.Enabled != nil && *oldShoot.Spec.Hibernation.Enabled
	newIsHibernated := shoot.Spec.Hibernation != nil && shoot.Spec.Hibernation.Enabled != nil && *shoot.Spec.Hibernation.Enabled

	if !oldIsHibernated && newIsHibernated {
		if hibernationConstraint := helper.GetCondition(shoot.Status.Constraints, core.ShootHibernationPossible); hibernationConstraint != nil {
			if hibernationConstraint.Status != core.ConditionTrue {
				return admission.NewForbidden(a, fmt.Errorf(hibernationConstraint.Message))
			}
		}
	}

	if seed != nil {
		if shoot.Spec.Networking.Pods == nil && seed.Spec.Networks.ShootDefaults != nil {
			shoot.Spec.Networking.Pods = seed.Spec.Networks.ShootDefaults.Pods
		}
		if shoot.Spec.Networking.Services == nil && seed.Spec.Networks.ShootDefaults != nil {
			shoot.Spec.Networking.Services = seed.Spec.Networks.ShootDefaults.Services
		}
	}

	// General approach with machine image defaulting in this code: Try to keep the machine image
	// from the old shoot object to not accidentally update it to the default machine image.
	// This should only happen in the maintenance time window of shoots and is performed by the
	// shoot maintenance controller.
	image, err := getDefaultMachineImage(cloudProfile.Spec.MachineImages)
	if err != nil {
		return apierrors.NewBadRequest(err.Error())
	}

	if !reflect.DeepEqual(oldShoot.Spec.Provider.InfrastructureConfig, shoot.Spec.Provider.InfrastructureConfig) {
		if shoot.ObjectMeta.Annotations == nil {
			shoot.ObjectMeta.Annotations = make(map[string]string)
		}
		controllerutils.AddTasks(shoot.ObjectMeta.Annotations, common.ShootTaskDeployInfrastructure)
	}

	for idx, worker := range shoot.Spec.Provider.Workers {
		if shoot.DeletionTimestamp == nil && worker.Machine.Image == nil {
			shoot.Spec.Provider.Workers[idx].Machine.Image = getOldWorkerMachineImageOrDefault(oldShoot.Spec.Provider.Workers, worker.Name, image)
		}
	}

	if seed != nil {
		if shoot.Spec.Networking.Pods == nil {
			if seed.Spec.Networks.ShootDefaults != nil {
				shoot.Spec.Networking.Pods = seed.Spec.Networks.ShootDefaults.Pods
			} else {
				allErrs = append(allErrs, field.Required(field.NewPath("spec", "networking", "pods"), "pods is required"))
			}
		}

		if shoot.Spec.Networking.Services == nil {
			if seed.Spec.Networks.ShootDefaults != nil {
				shoot.Spec.Networking.Services = seed.Spec.Networks.ShootDefaults.Services
			} else {
				allErrs = append(allErrs, field.Required(field.NewPath("spec", "networking", "services"), "services is required"))
			}
		}
	}

	allErrs = append(allErrs, validateProvider(validationContext)...)

	dnsErrors, err := validateDNSDomainUniqueness(v.shootLister, shoot.Name, shoot.Spec.DNS)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	allErrs = append(allErrs, dnsErrors...)

	if len(allErrs) > 0 {
		return admission.NewForbidden(a, fmt.Errorf("%+v", allErrs))
	}

	return nil
}

type validationContext struct {
	cloudProfile *core.CloudProfile
	seed         *core.Seed
	shoot        *core.Shoot
	oldShoot     *core.Shoot
}

func validateProvider(c *validationContext) field.ErrorList {
	var (
		allErrs = field.ErrorList{}
		path    = field.NewPath("spec", "provider")
	)

	if c.seed != nil {
		allErrs = append(allErrs, cidrvalidation.ValidateNetworkDisjointedness(
			path.Child("networks"),
			c.shoot.Spec.Networking.Nodes,
			c.shoot.Spec.Networking.Pods,
			c.shoot.Spec.Networking.Services,
			c.seed.Spec.Networks.Nodes,
			c.seed.Spec.Networks.Pods,
			c.seed.Spec.Networks.Services,
		)...)
	}

	ok, validKubernetesVersions, versionDefault := validateKubernetesVersionConstraints(c.cloudProfile.Spec.Kubernetes.Versions, c.shoot.Spec.Kubernetes.Version, c.oldShoot.Spec.Kubernetes.Version)
	if !ok {
		allErrs = append(allErrs, field.NotSupported(field.NewPath("spec", "kubernetes", "version"), c.shoot.Spec.Kubernetes.Version, validKubernetesVersions))
	} else if versionDefault != nil {
		c.shoot.Spec.Kubernetes.Version = versionDefault.String()
	}

	for i, worker := range c.shoot.Spec.Provider.Workers {
		var oldWorker = core.Worker{Machine: core.Machine{Image: &core.ShootMachineImage{}}}
		for _, ow := range c.oldShoot.Spec.Provider.Workers {
			if ow.Name == worker.Name {
				oldWorker = ow
				break
			}
		}

		idxPath := path.Child("workers").Index(i)
		if ok, validMachineTypes := validateMachineTypes(c.cloudProfile.Spec.MachineTypes, worker.Machine.Type, oldWorker.Machine.Type, c.cloudProfile.Spec.Regions, c.shoot.Spec.Region, worker.Zones); !ok {
			allErrs = append(allErrs, field.NotSupported(idxPath.Child("machine", "type"), worker.Machine.Type, validMachineTypes))
		}
		if ok, validMachineImages := validateMachineImagesConstraints(c.cloudProfile.Spec.MachineImages, worker.Machine.Image, oldWorker.Machine.Image); !ok {
			allErrs = append(allErrs, field.NotSupported(idxPath.Child("machine", "image"), worker.Machine.Image, validMachineImages))
		}
		if ok, validVolumeTypes := validateVolumeTypes(c.cloudProfile.Spec.VolumeTypes, worker.Volume, oldWorker.Volume, c.cloudProfile.Spec.Regions, c.shoot.Spec.Region, worker.Zones); !ok {
			allErrs = append(allErrs, field.NotSupported(idxPath.Child("volume", "type"), worker.Volume, validVolumeTypes))
		}

		allErrs = append(allErrs, validateZones(c.cloudProfile.Spec.Regions, c.shoot.Spec.Region, c.oldShoot.Spec.Region, worker, oldWorker, idxPath)...)
	}

	return allErrs
}

func validateDNSDomainUniqueness(shootLister corelisters.ShootLister, name string, dns *core.DNS) (field.ErrorList, error) {
	var (
		allErrs = field.ErrorList{}
		dnsPath = field.NewPath("spec", "dns", "domain")
	)

	if dns == nil || dns.Domain == nil {
		return allErrs, nil
	}

	shoots, err := shootLister.Shoots(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return allErrs, err
	}

	for _, shoot := range shoots {
		if shoot.Name == name {
			continue
		}

		var domain *string
		if shoot.Spec.DNS != nil {
			domain = shoot.Spec.DNS.Domain
		}
		if domain == nil {
			continue
		}

		// Prevent that this shoot uses the exact same domain of any other shoot in the system.
		if *domain == *dns.Domain {
			allErrs = append(allErrs, field.Duplicate(dnsPath, *dns.Domain))
			break
		}

		// Prevent that this shoot uses a subdomain of the domain of any other shoot in the system.
		if hasDomainIntersection(*domain, *dns.Domain) {
			allErrs = append(allErrs, field.Forbidden(dnsPath, "the domain is already used by another shoot or it is a subdomain of an already used domain"))
			break
		}
	}

	return allErrs, nil
}

// hasDomainIntersection checks if domainA is a suffix of domainB or domainB is a suffix of domainA.
func hasDomainIntersection(domainA, domainB string) bool {
	if domainA == domainB {
		return true
	}

	var short, long string
	if len(domainA) > len(domainB) {
		short = domainB
		long = domainA
	} else {
		short = domainA
		long = domainB
	}

	if !strings.HasPrefix(short, ".") {
		short = fmt.Sprintf(".%s", short)
	}

	return strings.HasSuffix(long, short)
}

func validateKubernetesVersionConstraints(constraints []core.ExpirableVersion, shootVersion, oldShootVersion string) (bool, []string, *semver.Version) {
	if shootVersion == oldShootVersion {
		return true, nil, nil
	}

	shootVersionSplit := strings.Split(shootVersion, ".")
	var (
		shootVersionMajor, shootVersionMinor int64
		getLatestPatchVersion                bool
	)
	if len(shootVersionSplit) == 2 {
		// add a fake patch version to avoid manual parsing
		fakeShootVersion := shootVersion + ".0"
		version, err := semver.NewVersion(fakeShootVersion)
		if err == nil {
			getLatestPatchVersion = true
			shootVersionMajor = version.Major()
			shootVersionMinor = version.Minor()
		}
	}

	validValues := []string{}
	var latestVersion *semver.Version
	for _, versionConstraint := range constraints {
		if versionConstraint.ExpirationDate != nil && versionConstraint.ExpirationDate.Time.Before(time.Now()) {
			continue
		}

		validValues = append(validValues, versionConstraint.Version)

		if versionConstraint.Version == shootVersion {
			return true, nil, nil
		}

		if getLatestPatchVersion {
			// CloudProfile cannot contain invalid semVer shootVersion
			cpVersion, _ := semver.NewVersion(versionConstraint.Version)

			if cpVersion.Major() != shootVersionMajor || cpVersion.Minor() != shootVersionMinor {
				continue
			}

			if latestVersion == nil || cpVersion.GreaterThan(latestVersion) {
				latestVersion = cpVersion
			}
		}
	}

	if latestVersion != nil {
		return true, nil, latestVersion
	}

	return false, validValues, nil
}

func validateMachineTypes(constraints []core.MachineType, machineType, oldMachineType string, regions []core.Region, region string, zones []string) (bool, []string) {
	if machineType == oldMachineType {
		return true, nil
	}

	validValues := []string{}

	var unavailableInAtLeastOneZone bool
top:
	for _, r := range regions {
		if r.Name != region {
			continue
		}

		for _, zoneName := range zones {
			for _, z := range r.Zones {
				if z.Name != zoneName {
					continue
				}

				for _, t := range z.UnavailableMachineTypes {
					if t == machineType {
						unavailableInAtLeastOneZone = true
						break top
					}
				}
			}
		}
	}

	for _, t := range constraints {
		if t.Usable != nil && !*t.Usable {
			continue
		}
		if unavailableInAtLeastOneZone {
			continue
		}
		validValues = append(validValues, t.Name)
		if t.Name == machineType {
			return true, nil
		}
	}

	return false, validValues
}

func validateVolumeTypes(constraints []core.VolumeType, volume, oldVolume *core.Volume, regions []core.Region, region string, zones []string) (bool, []string) {
	if volume == nil || volume.Type == nil || (volume != nil && oldVolume != nil && volume.Type != nil && oldVolume.Type != nil && *volume.Type == *oldVolume.Type) {
		return true, nil
	}

	var volumeType string
	if volume != nil && volume.Type != nil {
		volumeType = *volume.Type
	}

	validValues := []string{}

	var unavailableInAtLeastOneZone bool
top:
	for _, r := range regions {
		if r.Name != region {
			continue
		}

		for _, zoneName := range zones {
			for _, z := range r.Zones {
				if z.Name != zoneName {
					continue
				}

				for _, t := range z.UnavailableVolumeTypes {
					if t == volumeType {
						unavailableInAtLeastOneZone = true
						break top
					}
				}
			}
		}
	}

	for _, v := range constraints {
		if v.Usable != nil && !*v.Usable {
			continue
		}
		if unavailableInAtLeastOneZone {
			continue
		}
		validValues = append(validValues, v.Name)
		if v.Name == volumeType {
			return true, nil
		}
	}

	return false, validValues
}

func validateZones(constraints []core.Region, region, oldRegion string, worker, oldWorker core.Worker, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if region == oldRegion && reflect.DeepEqual(worker.Zones, oldWorker.Zones) {
		return allErrs
	}

	for j, zone := range worker.Zones {
		jdxPath := fldPath.Child("zones").Index(j)
		if ok, validZones := validateZone(constraints, region, zone); !ok {
			if len(validZones) == 0 {
				allErrs = append(allErrs, field.Invalid(jdxPath, region, "this region is not allowed"))
			} else {
				allErrs = append(allErrs, field.NotSupported(jdxPath, zone, validZones))
			}
		}
	}

	return allErrs
}

func validateZone(constraints []core.Region, region, zone string) (bool, []string) {
	validValues := []string{}

	for _, r := range constraints {
		if r.Name == region {
			for _, z := range r.Zones {
				validValues = append(validValues, z.Name)
				if z.Name == zone {
					return true, nil
				}
			}
			break
		}
	}

	return false, validValues
}

// getDefaultMachineImage determines the latest machine image version from the first machine image in the CloudProfile and considers that as the default image
func getDefaultMachineImage(machineImages []core.MachineImage) (*core.ShootMachineImage, error) {
	if len(machineImages) == 0 {
		return nil, errors.New("the cloud profile does not contain any machine image - cannot create shoot cluster")
	}
	firstMachineImageInCloudProfile := machineImages[0]
	latestMachineImageVersion, err := helper.DetermineLatestMachineImageVersion(firstMachineImageInCloudProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to determine latest machine image from cloud profile: %s", err.Error())
	}
	return &core.ShootMachineImage{Name: firstMachineImageInCloudProfile.Name, Version: latestMachineImageVersion.Version}, nil
}

func validateMachineImagesConstraints(constraints []core.MachineImage, image, oldImage *core.ShootMachineImage) (bool, []string) {
	if oldImage == nil || apiequality.Semantic.DeepEqual(image, oldImage) {
		return true, nil
	}

	validValues := []string{}
	if image == nil {
		for _, machineImage := range constraints {
			for _, machineVersion := range machineImage.Versions {
				if machineVersion.ExpirationDate != nil && machineVersion.ExpirationDate.Time.UTC().Before(time.Now().UTC()) {
					continue
				}
				validValues = append(validValues, fmt.Sprintf("machineImage(%s:%s)", machineImage.Name, machineVersion.Version))
			}
		}

		return false, validValues
	}

	for _, machineImage := range constraints {
		if machineImage.Name == image.Name {
			for _, machineVersion := range machineImage.Versions {
				if machineVersion.ExpirationDate != nil && machineVersion.ExpirationDate.Time.UTC().Before(time.Now().UTC()) {
					continue
				}
				validValues = append(validValues, fmt.Sprintf("machineImage(%s:%s)", machineImage.Name, machineVersion.Version))

				if machineVersion.Version == image.Version {
					return true, nil
				}
			}
		}
	}
	return false, validValues
}

func getOldWorkerMachineImageOrDefault(workers []core.Worker, name string, defaultImage *core.ShootMachineImage) *core.ShootMachineImage {
	if oldWorker := helper.FindWorkerByName(workers, name); oldWorker != nil && oldWorker.Machine.Image != nil {
		return oldWorker.Machine.Image
	}
	return defaultImage
}

func seedChanged(attr admission.Attributes) (bool, error) {
	if attr.GetOperation() != admission.Update {
		return false, nil
	}
	newShoot, ok := attr.GetObject().(*core.Shoot)
	if !ok {
		return false, errors.New("could not convert resource into Shoot object")
	}
	oldShoot, ok := attr.GetOldObject().(*core.Shoot)
	if !ok {
		return false, errors.New("could not convert old resource into Shoot object")
	}

	newSeedName := ""
	if newShoot.Spec.SeedName != nil {
		newSeedName = *newShoot.Spec.SeedName
	}
	oldSeedName := ""
	if oldShoot.Spec.SeedName != nil {
		oldSeedName = *oldShoot.Spec.SeedName
	}

	return newSeedName != oldSeedName, nil
}
