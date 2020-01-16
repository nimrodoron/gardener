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

package validation

import (
	"fmt"
	"net"
	"regexp"

	"github.com/gardener/gardener/pkg/apis/garden"
	"github.com/gardener/gardener/pkg/operation/common"
	cidrvalidation "github.com/gardener/gardener/pkg/utils/validation/cidr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
)

// ValidateSeed validates a Seed object.
func ValidateSeed(seed *garden.Seed) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateObjectMeta(&seed.ObjectMeta, false, ValidateName, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateSeedSpec(&seed.Spec, field.NewPath("spec"))...)
	allErrs = append(allErrs, ValidateSeedAnnotation(seed.ObjectMeta.Annotations, field.NewPath("metadata", "annotations"))...)

	return allErrs
}

// ValidateSeedUpdate validates a Seed object before an update.
func ValidateSeedUpdate(newSeed, oldSeed *garden.Seed) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateObjectMetaUpdate(&newSeed.ObjectMeta, &oldSeed.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateSeedSpecUpdate(&newSeed.Spec, &oldSeed.Spec, field.NewPath("spec"))...)
	allErrs = append(allErrs, ValidateSeed(newSeed)...)

	return allErrs
}

//ValidateSeedAnnotation validates the annotations of seed
func ValidateSeedAnnotation(annotations map[string]string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if annotations != nil {
		if v, ok := annotations[common.AnnotatePersistentVolumeMinimumSize]; ok {
			volumeSizeRegex, _ := regexp.Compile(`^(\d)+Gi$`)
			if !volumeSizeRegex.MatchString(v) {
				allErrs = append(allErrs, field.Invalid(fldPath.Key(common.AnnotatePersistentVolumeMinimumSize), v, fmt.Sprintf("volume size must match the regex %s", volumeSizeRegex)))
			}
		}
	}
	return allErrs
}

// ValidateSeedSpec validates the specification of a Seed object.
func ValidateSeedSpec(seedSpec *garden.SeedSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	cloudPath := fldPath.Child("cloud")
	if len(seedSpec.Cloud.Profile) == 0 {
		allErrs = append(allErrs, field.Required(cloudPath.Child("profile"), "must provide a cloud profile name"))
	}
	if len(seedSpec.Cloud.Region) == 0 {
		allErrs = append(allErrs, field.Required(cloudPath.Child("region"), "must provide a cloud region"))
	}

	providerPath := fldPath.Child("provider")
	if len(seedSpec.Provider.Type) == 0 {
		allErrs = append(allErrs, field.Required(providerPath.Child("type"), "must provide a provider type"))
	}
	if len(seedSpec.Provider.Region) == 0 {
		allErrs = append(allErrs, field.Required(providerPath.Child("region"), "must provide a provider region"))
	}

	allErrs = append(allErrs, validateDNS1123Subdomain(seedSpec.IngressDomain, fldPath.Child("ingressDomain"))...)
	if seedSpec.SecretRef != nil {
		allErrs = append(allErrs, validateSecretReference(*seedSpec.SecretRef, fldPath.Child("secretRef"))...)
	}

	networksPath := fldPath.Child("networks")

	networks := []cidrvalidation.CIDR{
		cidrvalidation.NewCIDR(seedSpec.Networks.Pods, networksPath.Child("pods")),
		cidrvalidation.NewCIDR(seedSpec.Networks.Services, networksPath.Child("services")),
	}
	if seedSpec.Networks.Nodes != nil {
		networks = append(networks, cidrvalidation.NewCIDR(*seedSpec.Networks.Nodes, networksPath.Child("nodes")))
	}
	if shootDefaults := seedSpec.Networks.ShootDefaults; shootDefaults != nil {
		if shootDefaults.Pods != nil {
			networks = append(networks, cidrvalidation.NewCIDR(*shootDefaults.Pods, networksPath.Child("shootDefaults", "pods")))
		}
		if shootDefaults.Services != nil {
			networks = append(networks, cidrvalidation.NewCIDR(*shootDefaults.Services, networksPath.Child("shootDefaults", "services")))
		}
	}

	allErrs = append(allErrs, cidrvalidation.ValidateCIDRParse(networks...)...)
	allErrs = append(allErrs, cidrvalidation.ValidateCIDROverlap(networks, networks, false)...)

	if seedSpec.Backup != nil {
		if len(seedSpec.Backup.Provider) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("backup", "provider"), "must provide a backup cloud provider name"))
		}

		// TOADD: Currently, getting cloud provider of seed requires fetching cloudProfile which requires gardenClient.
		// Hence we are not handling it here.
		// This should change with new `coreV1alpha1.Seed` api as per https://github.com/gardener/gardener/pull/1284/files#diff-bf2774d9954baab517306db45a5b80bbR241-R243,
		// and we will get direct seed cloud provider here.
		//
		//if seedSpec.Cloud.Type != seedSpec.Backup.Cloud &&( seedSpec.Backup.Region == nil || len(*seedSpec.Backup.Region) == 0) {
		//	allErrs = append(allErrs, field.Invalid(fldPath.Child("backup", "region"), "", "region must be specified for if backup provider is different from provider used in `spec.cloud`"))
		//}

		allErrs = append(allErrs, validateSecretReference(seedSpec.Backup.SecretRef, fldPath.Child("backup", "secretRef"))...)
	}

	var (
		supportedTaintKeys = sets.NewString(garden.SeedTaintDisableDNS, garden.SeedTaintProtected, garden.SeedTaintInvisible)
		foundTaintKeys     = sets.NewString()
	)

	for i, taint := range seedSpec.Taints {
		idxPath := fldPath.Child("taints").Index(i)
		if len(taint.Key) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("key"), "cannot be empty"))
		}
		if foundTaintKeys.Has(taint.Key) {
			allErrs = append(allErrs, field.Duplicate(idxPath.Child("key"), taint.Key))
		}
		if !supportedTaintKeys.Has(taint.Key) {
			allErrs = append(allErrs, field.NotSupported(idxPath.Child("key"), taint.Key, supportedTaintKeys.List()))
		}
		foundTaintKeys.Insert(taint.Key)
	}

	if seedSpec.Volume != nil {
		if seedSpec.Volume.MinimumSize != nil {
			allErrs = append(allErrs, validateResourceQuantityValue("minimumSize", *seedSpec.Volume.MinimumSize, fldPath.Child("volume", "minimumSize"))...)
		}

		volumeProviderPurposes := make(map[string]struct{}, len(seedSpec.Volume.Providers))
		for i, provider := range seedSpec.Volume.Providers {
			idxPath := fldPath.Child("volume", "providers").Index(i)
			if len(provider.Purpose) == 0 {
				allErrs = append(allErrs, field.Required(idxPath.Child("purpose"), "cannot be empty"))
			}
			if len(provider.Name) == 0 {
				allErrs = append(allErrs, field.Required(idxPath.Child("name"), "cannot be empty"))
			}
			if _, ok := volumeProviderPurposes[provider.Purpose]; ok {
				allErrs = append(allErrs, field.Duplicate(idxPath.Child("purpose"), provider.Purpose))
			}
			volumeProviderPurposes[provider.Purpose] = struct{}{}
		}
	}

	return allErrs
}

func validateCIDR(cidr string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if _, _, err := net.ParseCIDR(string(cidr)); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, cidr, err.Error()))
	}

	return allErrs
}

// ValidateSeedSpecUpdate validates the specification updates of a Seed object.
func ValidateSeedSpecUpdate(newSeedSpec, oldSeedSpec *garden.SeedSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSeedSpec.Networks.Pods, oldSeedSpec.Networks.Pods, fldPath.Child("networks", "pods"))...)
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSeedSpec.Networks.Services, oldSeedSpec.Networks.Services, fldPath.Child("networks", "services"))...)
	if oldSeedSpec.Networks.Nodes != nil {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSeedSpec.Networks.Nodes, oldSeedSpec.Networks.Nodes, fldPath.Child("networks", "nodes"))...)
	}

	if oldSeedSpec.Backup != nil {
		if newSeedSpec.Backup != nil {
			allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSeedSpec.Backup.Provider, oldSeedSpec.Backup.Provider, fldPath.Child("backup", "provider"))...)
			allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSeedSpec.Backup.Region, oldSeedSpec.Backup.Region, fldPath.Child("backup", "region"))...)
		} else {
			allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSeedSpec.Backup, oldSeedSpec.Backup, fldPath.Child("backup"))...)
		}
	}
	// If oldSeedSpec doesn't have backup configured, we allow to add it; but not the vice versa.

	return allErrs
}

// ValidateSeedStatusUpdate validates the status field of a Seed object.
func ValidateSeedStatusUpdate(newSeed, oldSeed *garden.Seed) field.ErrorList {
	allErrs := field.ErrorList{}

	return allErrs
}
