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

package helper_test

import (
	"github.com/gardener/gardener/pkg/apis/core"
	. "github.com/gardener/gardener/pkg/apis/core/helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("helper", func() {
	Describe("#GetCondition", func() {
		It("should return the found condition", func() {
			var (
				conditionType core.ConditionType = "test-1"
				condition                        = core.Condition{Type: conditionType}
				conditions                       = []core.Condition{condition}
			)

			cond := GetCondition(conditions, conditionType)

			Expect(cond).NotTo(BeNil())
			Expect(*cond).To(Equal(condition))
		})

		It("should return nil because the required condition could not be found", func() {
			var (
				conditionType core.ConditionType = "test-1"
				conditions                       = []core.Condition{}
			)

			cond := GetCondition(conditions, conditionType)

			Expect(cond).To(BeNil())
		})
	})

	DescribeTable("#QuotaScope",
		func(apiVersion, kind, expectedScope string, expectedErr gomegatypes.GomegaMatcher) {
			scope, err := QuotaScope(corev1.ObjectReference{APIVersion: apiVersion, Kind: kind})
			Expect(scope).To(Equal(expectedScope))
			Expect(err).To(expectedErr)
		},

		Entry("project", "core.gardener.cloud/v1beta1", "Project", "project", BeNil()),
		Entry("secret", "v1", "Secret", "secret", BeNil()),
		Entry("unknown", "v2", "Foo", "", HaveOccurred()),
	)

	var (
		trueVar  = true
		falseVar = false
	)

	DescribeTable("#ShootWantsBasicAuthentication",
		func(kubeAPIServerConfig *core.KubeAPIServerConfig, wantsBasicAuth bool) {
			actualWantsBasicAuth := ShootWantsBasicAuthentication(kubeAPIServerConfig)
			Expect(actualWantsBasicAuth).To(Equal(wantsBasicAuth))
		},

		Entry("no kubeapiserver configuration", nil, true),
		Entry("field not set", &core.KubeAPIServerConfig{}, true),
		Entry("explicitly enabled", &core.KubeAPIServerConfig{EnableBasicAuthentication: &trueVar}, true),
		Entry("explicitly disabled", &core.KubeAPIServerConfig{EnableBasicAuthentication: &falseVar}, false),
	)

	DescribeTable("#TaintsHave",
		func(taints []core.SeedTaint, key string, expectation bool) {
			Expect(TaintsHave(taints, key)).To(Equal(expectation))
		},

		Entry("taint exists", []core.SeedTaint{{Key: "foo"}}, "foo", true),
		Entry("taint does not exist", []core.SeedTaint{{Key: "foo"}}, "bar", false),
	)

	var (
		unmanagedType = core.DNSUnmanaged
		differentType = "foo"
	)

	DescribeTable("#ShootUsesUnmanagedDNS",
		func(dns *core.DNS, expectation bool) {
			shoot := &core.Shoot{
				Spec: core.ShootSpec{
					DNS: dns,
				},
			}
			Expect(ShootUsesUnmanagedDNS(shoot)).To(Equal(expectation))
		},

		Entry("no dns", nil, false),
		Entry("no dns providers", &core.DNS{}, false),
		Entry("dns providers but no type", &core.DNS{Providers: []core.DNSProvider{{}}}, false),
		Entry("dns providers but different type", &core.DNS{Providers: []core.DNSProvider{{Type: &differentType}}}, false),
		Entry("dns providers and unmanaged type", &core.DNS{Providers: []core.DNSProvider{{Type: &unmanagedType}}}, true),
	)

	DescribeTable("#FindWorkerByName",
		func(workers []core.Worker, name string, expectedWorker *core.Worker) {
			Expect(FindWorkerByName(workers, name)).To(Equal(expectedWorker))
		},

		Entry("no workers", nil, "", nil),
		Entry("worker not found", []core.Worker{{Name: "foo"}}, "bar", nil),
		Entry("worker found", []core.Worker{{Name: "foo"}}, "foo", &core.Worker{Name: "foo"}),
	)

	DescribeTable("#FindPrimaryDNSProvider",
		func(providers []core.DNSProvider, matcher gomegatypes.GomegaMatcher) {
			Expect(FindPrimaryDNSProvider(providers)).To(matcher)
		},

		Entry("no providers", nil, BeNil()),
		Entry("one non primary provider", []core.DNSProvider{{Type: pointer.StringPtr("provider")}}, BeNil()),
		Entry("one primary provider", []core.DNSProvider{{Type: pointer.StringPtr("provider"),
			Primary: pointer.BoolPtr(true)}}, Equal(&core.DNSProvider{Type: pointer.StringPtr("provider"), Primary: pointer.BoolPtr(true)})),
		Entry("multiple w/ one primary provider", []core.DNSProvider{
			{
				Type: pointer.StringPtr("provider2"),
			},
			{
				Type:    pointer.StringPtr("provider1"),
				Primary: pointer.BoolPtr(true),
			},
			{
				Type: pointer.StringPtr("provider3"),
			},
		}, Equal(&core.DNSProvider{Type: pointer.StringPtr("provider1"), Primary: pointer.BoolPtr(true)})),
		Entry("multiple w/ multiple primary providers", []core.DNSProvider{
			{
				Type:    pointer.StringPtr("provider1"),
				Primary: pointer.BoolPtr(true),
			},
			{
				Type:    pointer.StringPtr("provider2"),
				Primary: pointer.BoolPtr(true),
			},
			{
				Type: pointer.StringPtr("provider3"),
			},
		}, Equal(&core.DNSProvider{Type: pointer.StringPtr("provider1"), Primary: pointer.BoolPtr(true)})),
	)
})
