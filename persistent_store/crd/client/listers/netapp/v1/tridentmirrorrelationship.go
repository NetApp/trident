// Copyright 2023 NetApp, Inc. All Rights Reserved.

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

// TridentMirrorRelationshipLister helps list TridentMirrorRelationships.
type TridentMirrorRelationshipLister interface {
	// List lists all TridentMirrorRelationships in the indexer.
	List(selector labels.Selector) (ret []*v1.TridentMirrorRelationship, err error)
	// TridentMirrorRelationships returns an object that can list and get TridentMirrorRelationships.
	TridentMirrorRelationships(namespace string) TridentMirrorRelationshipNamespaceLister
	TridentMirrorRelationshipListerExpansion
}

// tridentMirrorRelationshipLister implements the TridentMirrorRelationshipLister interface.
type tridentMirrorRelationshipLister struct {
	indexer cache.Indexer
}

// NewTridentMirrorRelationshipLister returns a new TridentMirrorRelationshipLister.
func NewTridentMirrorRelationshipLister(indexer cache.Indexer) TridentMirrorRelationshipLister {
	return &tridentMirrorRelationshipLister{indexer: indexer}
}

// List lists all TridentMirrorRelationships in the indexer.
func (s *tridentMirrorRelationshipLister) List(selector labels.Selector) (ret []*v1.TridentMirrorRelationship, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.TridentMirrorRelationship))
	})
	return ret, err
}

// TridentMirrorRelationships returns an object that can list and get TridentMirrorRelationships.
func (s *tridentMirrorRelationshipLister) TridentMirrorRelationships(namespace string) TridentMirrorRelationshipNamespaceLister {
	return tridentMirrorRelationshipNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// TridentMirrorRelationshipNamespaceLister helps list and get TridentMirrorRelationships.
type TridentMirrorRelationshipNamespaceLister interface {
	// List lists all TridentMirrorRelationships in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1.TridentMirrorRelationship, err error)
	// Get retrieves the TridentMirrorRelationship from the indexer for a given namespace and name.
	Get(name string) (*v1.TridentMirrorRelationship, error)
	TridentMirrorRelationshipNamespaceListerExpansion
}

// tridentMirrorRelationshipNamespaceLister implements the TridentMirrorRelationshipNamespaceLister
// interface.
type tridentMirrorRelationshipNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all TridentMirrorRelationships in the indexer for a given namespace.
func (s tridentMirrorRelationshipNamespaceLister) List(selector labels.Selector) (ret []*v1.TridentMirrorRelationship, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.TridentMirrorRelationship))
	})
	return ret, err
}

// Get retrieves the TridentMirrorRelationship from the indexer for a given namespace and name.
func (s tridentMirrorRelationshipNamespaceLister) Get(name string) (*v1.TridentMirrorRelationship, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("tridentmirrorrelationship"), name)
	}
	return obj.(*v1.TridentMirrorRelationship), nil
}
