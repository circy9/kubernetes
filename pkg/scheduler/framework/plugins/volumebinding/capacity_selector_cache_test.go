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
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2/ktesting"
)

func TestCapacitySelectorCache(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	nodeA := labels.Set{nodeLabelKey: "node-a"}
	nodeB := labels.Set{nodeLabelKey: "node-b"}

	tests := []struct {
		name      string
		topology  *metav1.LabelSelector
		wantNil   bool
		wantNodeA bool
		wantNodeB bool
	}{
		{
			name:      "matchLabels",
			topology:  &metav1.LabelSelector{MatchLabels: map[string]string{nodeLabelKey: "node-a"}},
			wantNodeA: true,
		},
		{
			name: "matchExpressions",
			topology: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: nodeLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{"node-a", "node-b"}},
			}},
			wantNodeA: true,
			wantNodeB: true,
		},
		{
			name: "invalid",
			topology: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: nodeLabelKey, Operator: "BadOperator"},
			}},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c capacitySelectorCache
			capacity := &storagev1.CSIStorageCapacity{NodeTopology: tt.topology}
			selector := c.get(logger, capacity)
			if (selector == nil) != tt.wantNil {
				t.Fatalf("get() = %v, want nil: %v", selector, tt.wantNil)
			}
			if selector != nil {
				if got := selector.Matches(nodeA); got != tt.wantNodeA {
					t.Errorf("Matches(node-a) = %v, want %v", got, tt.wantNodeA)
				}
				if got := selector.Matches(nodeB); got != tt.wantNodeB {
					t.Errorf("Matches(node-b) = %v, want %v", got, tt.wantNodeB)
				}
			}
			if _, ok := c.selectors.Load(capacity); !ok {
				t.Fatal("selector was not cached")
			}
			if allocs := testing.AllocsPerRun(10, func() { c.get(logger, capacity) }); allocs != 0 {
				t.Errorf("cached get() allocated %v times, want 0", allocs)
			}
		})
	}
}

func TestCapacitySelectorCacheEventHandler(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	topology := &metav1.LabelSelector{MatchLabels: map[string]string{nodeLabelKey: "node-a"}}

	tests := []struct {
		name  string
		event func(h cache.ResourceEventHandler, old *storagev1.CSIStorageCapacity)
	}{
		{
			name: "update",
			event: func(h cache.ResourceEventHandler, old *storagev1.CSIStorageCapacity) {
				h.OnUpdate(old, old.DeepCopy())
			},
		},
		{
			name: "delete",
			event: func(h cache.ResourceEventHandler, old *storagev1.CSIStorageCapacity) {
				h.OnDelete(old)
			},
		},
		{
			name: "delete tombstone",
			event: func(h cache.ResourceEventHandler, old *storagev1.CSIStorageCapacity) {
				h.OnDelete(cache.DeletedFinalStateUnknown{Key: "ns/capacity", Obj: old})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c capacitySelectorCache
			old := &storagev1.CSIStorageCapacity{NodeTopology: topology}
			c.get(logger, old)
			tt.event(c.eventHandler(), old)
			if _, ok := c.selectors.Load(old); ok {
				t.Error("selector for the replaced object is still cached")
			}
		})
	}
}
