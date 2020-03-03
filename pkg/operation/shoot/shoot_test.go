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

package shoot_test

import (
	"context"
	"net"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"
	"github.com/gardener/gardener/pkg/operation/garden"
	. "github.com/gardener/gardener/pkg/operation/shoot"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("shoot", func() {
	Context("shoot", func() {
		var (
			ctrl *gomock.Controller
			c    *mockclient.MockClient

			shoot *Shoot
		)

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			c = mockclient.NewMockClient(ctrl)

			shoot = &Shoot{
				Info: &gardencorev1beta1.Shoot{},
			}
		})

		AfterEach(func() {
			ctrl.Finish()
		})

		Describe("#ToNetworks", func() {

			var shoot *gardencorev1beta1.Shoot

			BeforeEach(func() {
				shoot = &gardencorev1beta1.Shoot{
					Spec: gardencorev1beta1.ShootSpec{
						Networking: gardencorev1beta1.Networking{
							Pods:     pointer.StringPtr("10.0.0.0/24"),
							Services: pointer.StringPtr("20.0.0.0/24"),
						},
					},
				}
			})

			It("returns correct network", func() {
				result, err := ToNetworks(shoot)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(PointTo(Equal(Networks{
					Pods: &net.IPNet{
						IP:   []byte{10, 0, 0, 0},
						Mask: []byte{255, 255, 255, 0},
					},
					Services: &net.IPNet{
						IP:   []byte{20, 0, 0, 0},
						Mask: []byte{255, 255, 255, 0},
					},
					APIServer: []byte{20, 0, 0, 1},
					CoreDNS:   []byte{20, 0, 0, 10},
				})))
			})

			DescribeTable("#ConstructInternalClusterDomain", func(mutateFunc func(s *gardencorev1beta1.Shoot)) {
				mutateFunc(shoot)
				result, err := ToNetworks(shoot)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			},

				Entry("services is nil", func(s *gardencorev1beta1.Shoot) { s.Spec.Networking.Services = nil }),
				Entry("pods is nil", func(s *gardencorev1beta1.Shoot) { s.Spec.Networking.Pods = nil }),
				Entry("services is invalid", func(s *gardencorev1beta1.Shoot) {
					s.Spec.Networking.Services = pointer.StringPtr("foo")
				}),
				Entry("pods is invalid", func(s *gardencorev1beta1.Shoot) { s.Spec.Networking.Pods = pointer.StringPtr("foo") }),
				Entry("apiserver cannot be calculated", func(s *gardencorev1beta1.Shoot) {
					s.Spec.Networking.Services = pointer.StringPtr("10.0.0.0/32")
				}),
				Entry("coreDNS cannot be calculated", func(s *gardencorev1beta1.Shoot) {
					s.Spec.Networking.Services = pointer.StringPtr("10.0.0.0/29")
				}),
			)
		})

		Describe("#IPVSEnabled", func() {
			It("should return false when KubeProxy is null", func() {
				shoot.Info.Spec.Kubernetes.KubeProxy = nil
				Expect(shoot.IPVSEnabled()).To(BeFalse())
			})

			It("should return false when KubeProxy.Mode is null", func() {
				shoot.Info.Spec.Kubernetes.KubeProxy = &gardencorev1beta1.KubeProxyConfig{}
				Expect(shoot.IPVSEnabled()).To(BeFalse())
			})

			It("should return false when KubeProxy.Mode is not IPVS", func() {
				mode := gardencorev1beta1.ProxyModeIPTables
				shoot.Info.Spec.Kubernetes.KubeProxy = &gardencorev1beta1.KubeProxyConfig{
					Mode: &mode,
				}
				Expect(shoot.IPVSEnabled()).To(BeFalse())
			})

			It("should return true when KubeProxy.Mode is IPVS", func() {
				mode := gardencorev1beta1.ProxyModeIPVS
				shoot.Info.Spec.Kubernetes.KubeProxy = &gardencorev1beta1.KubeProxyConfig{
					Mode: &mode,
				}
				Expect(shoot.IPVSEnabled()).To(BeTrue())
			})
		})

		DescribeTable("#ConstructInternalClusterDomain",
			func(shootName, shootProject, internalDomain, expected string) {
				Expect(ConstructInternalClusterDomain(shootName, shootProject, &garden.Domain{Domain: internalDomain})).To(Equal(expected))
			},

			Entry("with internal domain key", "foo", "bar", "internal.nip.io", "foo.bar.internal.nip.io"),
			Entry("without internal domain key", "foo", "bar", "nip.io", "foo.bar.internal.nip.io"),
		)

		Describe("#ConstructExternalClusterDomain", func() {
			It("should return nil", func() {
				Expect(ConstructExternalClusterDomain(&gardencorev1beta1.Shoot{})).To(BeNil())
			})

			It("should return the constructed domain", func() {
				var (
					domain = "foo.bar.com"
					shoot  = &gardencorev1beta1.Shoot{
						Spec: gardencorev1beta1.ShootSpec{
							DNS: &gardencorev1beta1.DNS{
								Domain: &domain,
							},
						},
					}
				)

				Expect(ConstructExternalClusterDomain(shoot)).To(Equal(&domain))
			})
		})

		var (
			defaultDomainProvider   = "default-domain-provider"
			defaultDomainSecretData = map[string][]byte{"default": []byte("domain")}
			defaultDomain           = &garden.Domain{
				Domain:     "bar.com",
				Provider:   defaultDomainProvider,
				SecretData: defaultDomainSecretData,
			}
		)

		Describe("#ConstructExternalDomain", func() {
			var (
				namespace = "default"
				provider  = "my-dns-provider"
				domain    = "foo.bar.com"
			)

			It("returns nil because no external domain is used", func() {
				var (
					ctx   = context.TODO()
					shoot = &gardencorev1beta1.Shoot{}
				)

				externalDomain, err := ConstructExternalDomain(ctx, c, shoot, nil, nil)

				Expect(externalDomain).To(BeNil())
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the referenced secret", func() {
				var (
					ctx = context.TODO()

					dnsSecretName = "my-secret"
					dnsSecretData = map[string][]byte{"foo": []byte("bar")}
					dnsSecretKey  = kutil.Key(namespace, dnsSecretName)

					shoot = &gardencorev1beta1.Shoot{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
						},
						Spec: gardencorev1beta1.ShootSpec{
							DNS: &gardencorev1beta1.DNS{
								Domain: &domain,
								Providers: []gardencorev1beta1.DNSProvider{
									{
										Type:       &provider,
										SecretName: &dnsSecretName,
										Primary:    pointer.BoolPtr(true),
									},
								},
							},
						},
					}
				)

				c.EXPECT().Get(ctx, dnsSecretKey, gomock.AssignableToTypeOf(&corev1.Secret{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, secret *corev1.Secret) error {
					secret.Data = dnsSecretData
					return nil
				})

				externalDomain, err := ConstructExternalDomain(ctx, c, shoot, nil, nil)

				Expect(externalDomain).To(Equal(&garden.Domain{
					Domain:     domain,
					Provider:   provider,
					SecretData: dnsSecretData,
				}))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the default domain secret", func() {
				var (
					ctx = context.TODO()

					shoot = &gardencorev1beta1.Shoot{
						Spec: gardencorev1beta1.ShootSpec{
							DNS: &gardencorev1beta1.DNS{
								Domain: &domain,
								Providers: []gardencorev1beta1.DNSProvider{
									{
										Type: &provider,
									},
								},
							},
						},
					}
				)

				externalDomain, err := ConstructExternalDomain(ctx, c, shoot, nil, []*garden.Domain{defaultDomain})

				Expect(externalDomain).To(Equal(&garden.Domain{
					Domain:     domain,
					Provider:   defaultDomainProvider,
					SecretData: defaultDomainSecretData,
				}))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the shoot secret", func() {
				var (
					ctx = context.TODO()

					shootSecretData = map[string][]byte{"foo": []byte("bar")}
					shootSecret     = &corev1.Secret{Data: shootSecretData}
					shoot           = &gardencorev1beta1.Shoot{
						Spec: gardencorev1beta1.ShootSpec{
							DNS: &gardencorev1beta1.DNS{
								Domain: &domain,
								Providers: []gardencorev1beta1.DNSProvider{
									{
										Type:    &provider,
										Primary: pointer.BoolPtr(true),
									},
								},
							},
						},
					}
				)

				externalDomain, err := ConstructExternalDomain(ctx, c, shoot, shootSecret, nil)

				Expect(externalDomain).To(Equal(&garden.Domain{
					Domain:     domain,
					Provider:   provider,
					SecretData: shootSecretData,
				}))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("#ComputeInClusterAPIServerAddress", func() {
			seedNamespace := "foo"
			s := &Shoot{SeedNamespace: seedNamespace}

			It("should return <service-name>", func() {
				Expect(s.ComputeInClusterAPIServerAddress(true)).To(Equal(v1beta1constants.DeploymentNameKubeAPIServer))
			})

			It("should return <service-name>.<namespace>.svc", func() {
				Expect(s.ComputeInClusterAPIServerAddress(false)).To(Equal(v1beta1constants.DeploymentNameKubeAPIServer + "." + seedNamespace + ".svc"))
			})
		})

		Describe("#ComputeOutOfClusterAPIServerAddress", func() {
			It("should return the apiserver address as DNS is disabled", func() {
				s := &Shoot{DisableDNS: true}
				apiServerAddress := "abcd"

				Expect(s.ComputeOutOfClusterAPIServerAddress(apiServerAddress, false)).To(Equal(apiServerAddress))
			})

			It("should return the internal domain as shoot's external domain is unmanaged", func() {
				unmanaged := "unmanaged"
				internalDomain := "foo"
				s := &Shoot{
					InternalClusterDomain: internalDomain,
					Info: &gardencorev1beta1.Shoot{
						Spec: gardencorev1beta1.ShootSpec{
							DNS: &gardencorev1beta1.DNS{
								Providers: []gardencorev1beta1.DNSProvider{
									{Type: &unmanaged},
								},
							},
						},
					},
				}

				Expect(s.ComputeOutOfClusterAPIServerAddress("", false)).To(Equal("api." + internalDomain))
			})

			It("should return the internal domain as requested (shoot's external domain is not unmanaged)", func() {
				internalDomain := "foo"
				s := &Shoot{
					InternalClusterDomain: internalDomain,
					Info:                  &gardencorev1beta1.Shoot{},
				}

				Expect(s.ComputeOutOfClusterAPIServerAddress("", true)).To(Equal("api." + internalDomain))
			})

			It("should return the external domain as requested (shoot's external domain is not unmanaged)", func() {
				externalDomain := "foo"
				s := &Shoot{
					ExternalClusterDomain: &externalDomain,
					Info:                  &gardencorev1beta1.Shoot{},
				}

				Expect(s.ComputeOutOfClusterAPIServerAddress("", false)).To(Equal("api." + externalDomain))
			})
		})

		Describe("#UnfoldTechnicalID", func() {
			DescribeTable("", func(technicalId string, projectNameMatcher types.GomegaMatcher, matcher types.GomegaMatcher) {
				projectName, shootName := UnfoldTechnicalID(technicalId)
				Expect(projectName).To(projectNameMatcher)
				Expect(shootName).To(matcher)
			},
				Entry("returns empty strings for provided zero length string", "", BeEmpty(), BeEmpty()),
				Entry("returns empty strings, invalid technicalID", "invalidstring", BeEmpty(), BeEmpty()),
				Entry("valid technicalID", "shoot--project-name--shoot-name", Equal("project-name"), Equal("shoot-name")),
				Entry("valid technicalID for deprecated project and shoot naming", "shoot-projectname-shootname", Equal("projectname"), Equal("shootname")),
			)
		})
	})

	Context("Extensions", func() {
		var (
			shootNamespace = "shoot--foo--bar"
			extensionKind  = extensionsv1alpha1.ExtensionResource
			providerConfig = gardencorev1beta1.ProviderConfig{
				RawExtension: runtime.RawExtension{
					Raw: []byte("key: value"),
				},
			}
			fooExtensionType         = "foo"
			fooReconciliationTimeout = metav1.Duration{Duration: 5 * time.Minute}
			fooRegistration          = gardencorev1beta1.ControllerRegistration{
				Spec: gardencorev1beta1.ControllerRegistrationSpec{
					Resources: []gardencorev1beta1.ControllerResource{
						{
							Kind:             extensionKind,
							Type:             fooExtensionType,
							ReconcileTimeout: &fooReconciliationTimeout,
						},
					},
				},
			}
			fooExtension = gardencorev1beta1.Extension{
				Type:           fooExtensionType,
				ProviderConfig: &providerConfig,
			}
			barExtensionType = "bar"
			barRegistration  = gardencorev1beta1.ControllerRegistration{
				Spec: gardencorev1beta1.ControllerRegistrationSpec{
					Resources: []gardencorev1beta1.ControllerResource{
						{
							Kind:            extensionKind,
							Type:            barExtensionType,
							GloballyEnabled: pointer.BoolPtr(true),
						},
					},
				},
			}
			barExtension = gardencorev1beta1.Extension{
				Type:           barExtensionType,
				ProviderConfig: &providerConfig,
			}
		)

		DescribeTable("#MergeExtensions",
			func(registrations []gardencorev1beta1.ControllerRegistration, extensions []gardencorev1beta1.Extension, namespace string, conditionMatcher types.GomegaMatcher) {
				ext, err := MergeExtensions(registrations, extensions, namespace)
				Expect(ext).To(conditionMatcher)
				Expect(err).To(BeNil())
			},
			Entry("No extensions", nil, nil, shootNamespace, BeEmpty()),
			Entry("Extension w/o registration", nil, []gardencorev1beta1.Extension{{Type: fooExtensionType}}, shootNamespace, BeEmpty()),
			Entry("Extensions w/ registration",
				[]gardencorev1beta1.ControllerRegistration{
					fooRegistration,
				},
				[]gardencorev1beta1.Extension{
					fooExtension,
				},
				shootNamespace,
				HaveKeyWithValue(
					Equal(fooExtensionType),
					MatchAllFields(
						Fields{
							"Extension": MatchFields(IgnoreExtras, Fields{
								"Spec": MatchFields(IgnoreExtras, Fields{
									"DefaultSpec": MatchAllFields(Fields{
										"Type":           Equal(fooExtensionType),
										"ProviderConfig": PointTo(Equal(providerConfig.RawExtension)),
									}),
								}),
							}),
							"Timeout": Equal(fooReconciliationTimeout.Duration),
						},
					),
				),
			),
			Entry("Registration w/o extension",
				[]gardencorev1beta1.ControllerRegistration{
					fooRegistration,
				},
				nil,
				shootNamespace,
				BeEmpty(),
			),
			Entry("Required extension registration, w/o extension",
				[]gardencorev1beta1.ControllerRegistration{
					barRegistration,
				},
				nil,
				shootNamespace,
				HaveKeyWithValue(
					Equal(barExtensionType),
					MatchAllFields(
						Fields{
							"Extension": MatchFields(IgnoreExtras, Fields{
								"Spec": MatchAllFields(Fields{
									"DefaultSpec": MatchAllFields(Fields{
										"Type":           Equal(barExtensionType),
										"ProviderConfig": BeNil(),
									}),
								}),
							}),
							"Timeout": Equal(ExtensionDefaultTimeout),
						},
					),
				),
			),
			Entry("Multuple registrations, w/ one extension",
				[]gardencorev1beta1.ControllerRegistration{
					fooRegistration,
					barRegistration,
					{
						Spec: gardencorev1beta1.ControllerRegistrationSpec{
							Resources: []gardencorev1beta1.ControllerResource{
								{
									Kind: "kind",
									Type: "type",
								},
							},
						},
					},
				},
				[]gardencorev1beta1.Extension{
					barExtension,
				},
				shootNamespace,
				HaveKeyWithValue(
					Equal(barExtensionType),
					MatchAllFields(
						Fields{
							"Extension": MatchFields(IgnoreExtras, Fields{
								"Spec": MatchAllFields(Fields{
									"DefaultSpec": MatchAllFields(Fields{
										"Type":           Equal(barExtensionType),
										"ProviderConfig": PointTo(Equal(providerConfig.RawExtension)),
									}),
								}),
							}),
							"Timeout": Equal(ExtensionDefaultTimeout),
						},
					),
				),
			),
		)
	})
})
