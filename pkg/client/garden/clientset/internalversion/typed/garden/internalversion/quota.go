/*
Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

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

// Code generated by client-gen. DO NOT EDIT.

package internalversion

import (
	"time"

	garden "github.com/gardener/gardener/pkg/apis/garden"
	scheme "github.com/gardener/gardener/pkg/client/garden/clientset/internalversion/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// QuotasGetter has a method to return a QuotaInterface.
// A group's client should implement this interface.
type QuotasGetter interface {
	Quotas(namespace string) QuotaInterface
}

// QuotaInterface has methods to work with Quota resources.
type QuotaInterface interface {
	Create(*garden.Quota) (*garden.Quota, error)
	Update(*garden.Quota) (*garden.Quota, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*garden.Quota, error)
	List(opts v1.ListOptions) (*garden.QuotaList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *garden.Quota, err error)
	QuotaExpansion
}

// quotas implements QuotaInterface
type quotas struct {
	client rest.Interface
	ns     string
}

// newQuotas returns a Quotas
func newQuotas(c *GardenClient, namespace string) *quotas {
	return &quotas{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the quota, and returns the corresponding quota object, and an error if there is any.
func (c *quotas) Get(name string, options v1.GetOptions) (result *garden.Quota, err error) {
	result = &garden.Quota{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("quotas").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Quotas that match those selectors.
func (c *quotas) List(opts v1.ListOptions) (result *garden.QuotaList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &garden.QuotaList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("quotas").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested quotas.
func (c *quotas) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("quotas").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a quota and creates it.  Returns the server's representation of the quota, and an error, if there is any.
func (c *quotas) Create(quota *garden.Quota) (result *garden.Quota, err error) {
	result = &garden.Quota{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("quotas").
		Body(quota).
		Do().
		Into(result)
	return
}

// Update takes the representation of a quota and updates it. Returns the server's representation of the quota, and an error, if there is any.
func (c *quotas) Update(quota *garden.Quota) (result *garden.Quota, err error) {
	result = &garden.Quota{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("quotas").
		Name(quota.Name).
		Body(quota).
		Do().
		Into(result)
	return
}

// Delete takes name of the quota and deletes it. Returns an error if one occurs.
func (c *quotas) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("quotas").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *quotas) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("quotas").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched quota.
func (c *quotas) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *garden.Quota, err error) {
	result = &garden.Quota{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("quotas").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
