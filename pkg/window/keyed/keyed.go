/*
Copyright 2022 The Numaproj Authors.

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

// Package keyed implements KeyedWindows. A keyed window associates key(s) with a window.
// A key uniquely identifies a partitioned set of events in a given time window.
package keyed

import (
	"sync"

	"github.com/numaproj/numaflow/pkg/pbq/partition"
	"github.com/numaproj/numaflow/pkg/window/strategy"
)

// KeyedWindow maintains association between keys and a window.
// In a keyed stream, we need to close all the partitions when the watermark is past the window.
type KeyedWindow struct {
	strategy.AlignedWindow
	Keys map[string]struct{}
	lock sync.RWMutex
}

// NewKeyedWindow creates a new keyed window
func NewKeyedWindow(window strategy.AlignedWindow) *KeyedWindow {
	kw := &KeyedWindow{
		AlignedWindow: window,
		Keys:          make(map[string]struct{}),
		lock:          sync.RWMutex{},
	}
	return kw
}

// AddKey adds a key to an existing window
func (kw *KeyedWindow) AddKey(key string) {
	kw.lock.Lock()
	defer kw.lock.Unlock()
	if _, ok := kw.Keys[key]; !ok {
		kw.Keys[key] = struct{}{}
	}
}

// Partitions returns an array of partitions for a window
func (kw *KeyedWindow) Partitions() []partition.ID {
	kw.lock.RLock()
	defer kw.lock.RUnlock()

	partitions := make([]partition.ID, len(kw.Keys))
	idx := 0
	for k := range kw.Keys {
		partitions[idx] = partition.ID{Start: kw.StartTime(), End: kw.EndTime(), Key: k}
		idx++
	}

	return partitions
}
