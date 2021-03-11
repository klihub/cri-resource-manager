// Copyright 2021 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package idxset

import (
	"fmt"
	"sort"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

//
// sparse implements IdxSet as a map with one entry per each index present
// in the set. It can be used to efficiently represent sparse sets, where
// only a small number of indexes from a large values space is expected to
// be present in each set instace.
type sparse struct {
	members map[int]struct{} // indexes in the set
	indices []int            // cached slice of sorted indices
	str     string           // cached result of String()
	cpuset  *cpuset.CPUSet   // cached result of CPUSet()
}

// NewSparseSet creates a new sparse index set with the given indices.
func NewSparseSet(indices ...int) IdxSet {
	return newSparseSet(indices...)
}

// ParseSparseSet parses the given string into a sparse index set.
func ParseSparseSet(str string) (IdxSet, error) {
	s := newSparseSet()
	if err := s.Parse(str); err != nil {
		return nil, err
	}
	return s, nil
}

// MustParseSparseSet parses the given string into a sparse index set.
func MustParseSparseSet(str string) IdxSet {
	s := newSparseSet()
	return s.MustParse(str)
}

func (s *sparse) Clone() IdxSet {
	return s.clone()
}

func (s *sparse) Reset() IdxSet {
	*s = *newSparseSet()
	return s
}

func (s *sparse) Size() int {
	return len(s.members)
}

func (s *sparse) Indices() []int {
	if s.indices == nil {
		s.indices = make([]int, 0, len(s.members))
		for idx := range s.members {
			s.indices = append(s.indices, idx)
		}
		sort.Ints(s.indices)
	}
	return s.indices
}

func (s *sparse) Add(indices ...int) IdxSet {
	altered := false
	for _, idx := range indices {
		if idx < 0 {
			err := idxsetError("invalid (negative) index: %d", idx)
			panic(fmt.Sprintf("%v", err))
		}
		s.members[idx] = struct{}{}
		altered = true
	}
	s.altered(altered)

	return s
}

func (s *sparse) Del(indices ...int) IdxSet {
	size := len(s.members)
	for _, idx := range indices {
		if idx < 0 {
			err := idxsetError("invalid (negative) index: %d", idx)
			panic(fmt.Sprintf("%v", err))
		}
		delete(s.members, idx)
	}
	s.altered(len(s.members) != size)

	return s
}

func (s *sparse) Contains(indices ...int) bool {
	for _, idx := range indices {
		if idx < 0 {
			err := idxsetError("invalid (negative) index: %d", idx)
			panic(fmt.Sprintf("%v", err))
		}
		if _, ok := s.members[idx]; !ok {
			return false
		}
	}
	return true
}

func (s *sparse) Union(other IdxSet) IdxSet {
	var r IdxSet

	if o, ok := other.(*sparse); ok {
		r = s.union(o)
	} else {
		r = s.clone().Add(other.Indices()...).(*sparse)
	}

	return r
}

func (s *sparse) Unite(other IdxSet) IdxSet {
	if o, ok := other.(*sparse); ok {
		return s.unite(o)
	} else {
		s.Add(other.Indices()...)
	}

	return s
}

func (s *sparse) Intersection(other IdxSet) IdxSet {
	var r IdxSet

	if o, ok := other.(*sparse); ok {
		r = s.intersection(o)
	} else {
		indices := []int{}
		for _, idx := range other.Indices() {
			if s.Contains(idx) {
				indices = append(indices, idx)
			}
		}
		r = newSparseSet(indices...)
	}

	return r
}

func (s *sparse) Intersect(other IdxSet) IdxSet {
	if o, ok := other.(*sparse); ok {
		s.intersect(o)
	} else {
		var indices []int
		for _, idx := range other.Indices() {
			if s.Contains(idx) {
				indices = append(indices, idx)
			}
		}
		*s = *newSparseSet(indices...)
	}

	return s
}

func (s *sparse) Difference(other IdxSet) IdxSet {
	var r IdxSet

	if o, ok := other.(*sparse); ok {
		r = s.difference(o)
	} else {
		r = s.clone()
		r.Del(other.Indices()...)
	}

	return r
}

func (s *sparse) Subtract(other IdxSet) IdxSet {
	if o, ok := other.(*sparse); ok {
		s.subtract(o)
	} else {
		s.Del(other.Indices()...)
	}

	return s
}

func (s *sparse) Equals(other IdxSet) bool {
	var r bool

	if o, ok := other.(*sparse); ok {
		r = s.equals(o)
	} else {
		indices := other.Indices()
		if !s.Contains(indices...) {
			r = false
		} else {
			r = len(s.Indices()) == len(indices)
		}
	}

	return r
}

func (s *sparse) String() string {
	if s.str == "" {
		s.str = toString(s)
	}
	return s.str
}

func (s *sparse) Parse(str string) error {
	return parse(s, str)
}

func (s *sparse) MustParse(str string) IdxSet {
	return mustParse(s, str)
}

func (s *sparse) ForEach(fn func(int) bool) {
	for _, idx := range s.Indices() {
		if fn(idx) {
			return
		}
	}
}

func (s *sparse) CPUSet() cpuset.CPUSet {
	if s.cpuset == nil {
		cset := cpuset.NewCPUSet(s.Indices()...)
		s.cpuset = &cset
	}
	return *s.cpuset
}

func newSparseSet(indices ...int) *sparse {
	s := &sparse{members: make(map[int]struct{})}
	for _, idx := range indices {
		s.members[idx] = struct{}{}
	}
	return s
}

func (s *sparse) clone() *sparse {
	c := &sparse{
		members: make(map[int]struct{}),
		str:     s.str,
		cpuset:  s.cpuset,
	}
	for idx := range s.members {
		c.members[idx] = struct{}{}
	}
	return c
}

func (s *sparse) union(o *sparse) *sparse {
	r := &sparse{members: make(map[int]struct{})}

	for idx := range s.members {
		r.members[idx] = struct{}{}
	}
	for idx := range o.members {
		r.members[idx] = struct{}{}
	}

	return r
}

func (s *sparse) unite(o *sparse) *sparse {
	altered := false
	for idx := range o.members {
		s.members[idx] = struct{}{}
		altered = true
	}
	s.altered(altered)
	return s
}

func (s *sparse) intersection(o *sparse) *sparse {
	r := &sparse{members: make(map[int]struct{})}

	for idx := range s.members {
		if _, ok := o.members[idx]; ok {
			r.members[idx] = struct{}{}
		}
	}

	return r
}

func (s *sparse) intersect(o *sparse) *sparse {
	altered := false
	for idx := range s.members {
		if _, ok := o.members[idx]; !ok {
			delete(s.members, idx)
			altered = true
		}
	}
	s.altered(altered)
	return s
}

func (s *sparse) difference(o *sparse) *sparse {
	r := &sparse{members: make(map[int]struct{})}

	for idx := range s.members {
		if _, ok := o.members[idx]; !ok {
			r.members[idx] = struct{}{}
		}
	}

	return r
}

func (s *sparse) subtract(o *sparse) *sparse {
	altered := false
	for idx := range s.members {
		if _, ok := o.members[idx]; ok {
			delete(s.members, idx)
			altered = true
		}
	}
	s.altered(altered)

	return s
}

func (s *sparse) equals(o *sparse) bool {
	if len(s.members) != len(o.members) {
		return false
	}
	for idx := range s.members {
		if _, ok := o.members[idx]; !ok {
			return false
		}
	}
	return true
}

func (s *sparse) altered(altered bool) {
	if altered {
		s.indices = nil
		s.str = ""
		s.cpuset = nil
	}
}
