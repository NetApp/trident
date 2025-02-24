// Copyright 2023 NetApp, Inc. All Rights Reserved.

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"

	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

// FakeTridentStorageClasses implements TridentStorageClassInterface
type FakeTridentStorageClasses struct {
	Fake *FakeTridentV1
	ns   string
}

var tridentstorageclassesResource = schema.GroupVersionResource{Group: "trident.netapp.io", Version: "v1", Resource: "tridentstorageclasses"}

var tridentstorageclassesKind = schema.GroupVersionKind{Group: "trident.netapp.io", Version: "v1", Kind: "TridentStorageClass"}

// Get takes name of the tridentStorageClass, and returns the corresponding tridentStorageClass object, and an error if there is any.
func (c *FakeTridentStorageClasses) Get(ctx context.Context, name string, options v1.GetOptions) (result *netappv1.TridentStorageClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(tridentstorageclassesResource, c.ns, name), &netappv1.TridentStorageClass{})

	if obj == nil {
		return nil, err
	}
	return obj.(*netappv1.TridentStorageClass), err
}

// List takes label and field selectors, and returns the list of TridentStorageClasses that match those selectors.
func (c *FakeTridentStorageClasses) List(ctx context.Context, opts v1.ListOptions) (result *netappv1.TridentStorageClassList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(tridentstorageclassesResource, tridentstorageclassesKind, c.ns, opts), &netappv1.TridentStorageClassList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &netappv1.TridentStorageClassList{ListMeta: obj.(*netappv1.TridentStorageClassList).ListMeta}
	for _, item := range obj.(*netappv1.TridentStorageClassList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested tridentStorageClasses.
func (c *FakeTridentStorageClasses) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(tridentstorageclassesResource, c.ns, opts))

}

// Create takes the representation of a tridentStorageClass and creates it.  Returns the server's representation of the tridentStorageClass, and an error, if there is any.
func (c *FakeTridentStorageClasses) Create(ctx context.Context, tridentStorageClass *netappv1.TridentStorageClass, opts v1.CreateOptions) (result *netappv1.TridentStorageClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(tridentstorageclassesResource, c.ns, tridentStorageClass), &netappv1.TridentStorageClass{})

	if obj == nil {
		return nil, err
	}
	return obj.(*netappv1.TridentStorageClass), err
}

// Update takes the representation of a tridentStorageClass and updates it. Returns the server's representation of the tridentStorageClass, and an error, if there is any.
func (c *FakeTridentStorageClasses) Update(ctx context.Context, tridentStorageClass *netappv1.TridentStorageClass, opts v1.UpdateOptions) (result *netappv1.TridentStorageClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(tridentstorageclassesResource, c.ns, tridentStorageClass), &netappv1.TridentStorageClass{})

	if obj == nil {
		return nil, err
	}
	return obj.(*netappv1.TridentStorageClass), err
}

// Delete takes name of the tridentStorageClass and deletes it. Returns an error if one occurs.
func (c *FakeTridentStorageClasses) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(tridentstorageclassesResource, c.ns, name), &netappv1.TridentStorageClass{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeTridentStorageClasses) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(tridentstorageclassesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &netappv1.TridentStorageClassList{})
	return err
}

// Patch applies the patch and returns the patched tridentStorageClass.
func (c *FakeTridentStorageClasses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *netappv1.TridentStorageClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(tridentstorageclassesResource, c.ns, name, pt, data, subresources...), &netappv1.TridentStorageClass{})

	if obj == nil {
		return nil, err
	}
	return obj.(*netappv1.TridentStorageClass), err
}
