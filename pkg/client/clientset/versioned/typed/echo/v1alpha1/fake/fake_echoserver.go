/*
Copyright The Kubernetes Authors.

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

package fake

import (
	v1alpha1 "github.com/objectrocket/tiny-operator/pkg/apis/echo/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeEchoServers implements EchoServerInterface
type FakeEchoServers struct {
	Fake *FakeEchoV1alpha1
	ns   string
}

var echoserversResource = schema.GroupVersionResource{Group: "echo", Version: "v1alpha1", Resource: "echoservers"}

var echoserversKind = schema.GroupVersionKind{Group: "echo", Version: "v1alpha1", Kind: "EchoServer"}

// Get takes name of the echoServer, and returns the corresponding echoServer object, and an error if there is any.
func (c *FakeEchoServers) Get(name string, options v1.GetOptions) (result *v1alpha1.EchoServer, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(echoserversResource, c.ns, name), &v1alpha1.EchoServer{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.EchoServer), err
}

// List takes label and field selectors, and returns the list of EchoServers that match those selectors.
func (c *FakeEchoServers) List(opts v1.ListOptions) (result *v1alpha1.EchoServerList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(echoserversResource, echoserversKind, c.ns, opts), &v1alpha1.EchoServerList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.EchoServerList{}
	for _, item := range obj.(*v1alpha1.EchoServerList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested echoServers.
func (c *FakeEchoServers) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(echoserversResource, c.ns, opts))

}

// Create takes the representation of a echoServer and creates it.  Returns the server's representation of the echoServer, and an error, if there is any.
func (c *FakeEchoServers) Create(echoServer *v1alpha1.EchoServer) (result *v1alpha1.EchoServer, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(echoserversResource, c.ns, echoServer), &v1alpha1.EchoServer{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.EchoServer), err
}

// Update takes the representation of a echoServer and updates it. Returns the server's representation of the echoServer, and an error, if there is any.
func (c *FakeEchoServers) Update(echoServer *v1alpha1.EchoServer) (result *v1alpha1.EchoServer, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(echoserversResource, c.ns, echoServer), &v1alpha1.EchoServer{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.EchoServer), err
}

// Delete takes name of the echoServer and deletes it. Returns an error if one occurs.
func (c *FakeEchoServers) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(echoserversResource, c.ns, name), &v1alpha1.EchoServer{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeEchoServers) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(echoserversResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.EchoServerList{})
	return err
}

// Patch applies the patch and returns the patched echoServer.
func (c *FakeEchoServers) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.EchoServer, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(echoserversResource, c.ns, name, data, subresources...), &v1alpha1.EchoServer{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.EchoServer), err
}
