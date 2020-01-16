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

package v1alpha1

import (
	"time"

	v1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	scheme "github.com/gardener/gardener/pkg/client/extensions/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// NetworksGetter has a method to return a NetworkInterface.
// A group's client should implement this interface.
type NetworksGetter interface {
	Networks(namespace string) NetworkInterface
}

// NetworkInterface has methods to work with Network resources.
type NetworkInterface interface {
	Create(*v1alpha1.Network) (*v1alpha1.Network, error)
	Update(*v1alpha1.Network) (*v1alpha1.Network, error)
	UpdateStatus(*v1alpha1.Network) (*v1alpha1.Network, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.Network, error)
	List(opts v1.ListOptions) (*v1alpha1.NetworkList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Network, err error)
	NetworkExpansion
}

// networks implements NetworkInterface
type networks struct {
	client rest.Interface
	ns     string
}

// newNetworks returns a Networks
func newNetworks(c *ExtensionsV1alpha1Client, namespace string) *networks {
	return &networks{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the network, and returns the corresponding network object, and an error if there is any.
func (c *networks) Get(name string, options v1.GetOptions) (result *v1alpha1.Network, err error) {
	result = &v1alpha1.Network{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("networks").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Networks that match those selectors.
func (c *networks) List(opts v1.ListOptions) (result *v1alpha1.NetworkList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.NetworkList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("networks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested networks.
func (c *networks) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("networks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a network and creates it.  Returns the server's representation of the network, and an error, if there is any.
func (c *networks) Create(network *v1alpha1.Network) (result *v1alpha1.Network, err error) {
	result = &v1alpha1.Network{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("networks").
		Body(network).
		Do().
		Into(result)
	return
}

// Update takes the representation of a network and updates it. Returns the server's representation of the network, and an error, if there is any.
func (c *networks) Update(network *v1alpha1.Network) (result *v1alpha1.Network, err error) {
	result = &v1alpha1.Network{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("networks").
		Name(network.Name).
		Body(network).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *networks) UpdateStatus(network *v1alpha1.Network) (result *v1alpha1.Network, err error) {
	result = &v1alpha1.Network{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("networks").
		Name(network.Name).
		SubResource("status").
		Body(network).
		Do().
		Into(result)
	return
}

// Delete takes name of the network and deletes it. Returns an error if one occurs.
func (c *networks) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("networks").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *networks) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("networks").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched network.
func (c *networks) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.Network, err error) {
	result = &v1alpha1.Network{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("networks").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
