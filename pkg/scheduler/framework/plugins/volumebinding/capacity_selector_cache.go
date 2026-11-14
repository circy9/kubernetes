/*
Copyright 2026 The Kubernetes Authors.

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

package volumebinding

import (
	"sync"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// capacitySelectorCache caches the label selector built from each
// CSIStorageCapacity's NodeTopology, so that Filter doesn't rebuild it for
// every node.
//
// Entries are keyed by the object pointer returned by the informer's lister.
// Informer caches never modify objects in place: an update stores a new
// object. A pointer therefore identifies one version of one object, and its
// selector can't go stale. Update and delete events drop the entry for the
// replaced object so the cache doesn't grow.
type capacitySelectorCache struct {
	// selectors maps *storagev1.CSIStorageCapacity to labels.Selector.
	// A nil selector means NodeTopology couldn't be converted.
	selectors sync.Map
}

// get returns the selector for capacity.NodeTopology, building and caching it
// on first use. It returns nil if the topology is invalid.
func (c *capacitySelectorCache) get(logger klog.Logger, capacity *storagev1.CSIStorageCapacity) labels.Selector {
	if s, ok := c.selectors.Load(capacity); ok {
		selector, _ := s.(labels.Selector)
		return selector
	}
	selector, err := metav1.LabelSelectorAsSelector(capacity.NodeTopology)
	if err != nil {
		logger.Error(err, "Unexpected error converting to a label selector", "nodeTopology", capacity.NodeTopology)
		selector = nil
	}
	c.selectors.Store(capacity, selector)
	return selector
}

// forget drops the cached selector for obj, which may be a
// *storagev1.CSIStorageCapacity or a cache.DeletedFinalStateUnknown wrapping one.
func (c *capacitySelectorCache) forget(obj interface{}) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	if capacity, ok := obj.(*storagev1.CSIStorageCapacity); ok {
		c.selectors.Delete(capacity)
	}
}

// eventHandler keeps the cache in sync with the CSIStorageCapacity informer.
func (c *capacitySelectorCache) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, _ interface{}) { c.forget(oldObj) },
		DeleteFunc: c.forget,
	}
}
