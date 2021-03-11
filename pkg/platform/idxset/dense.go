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
	"math/bits"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

//
// dense implements IdxSet as a slice of uint64 masks. For every index IDX it
// uses bit (1 << (IDX&63)) within mask[IDX/64] to store if IDX is present in
// the set (bit 1) or absent (bit 0). It can be used to efficiently represent
// large dense sets, IOW sets, where a high percentage between the lowest and
// highest indices of the set are part of the set.
//
// Notes:
//   Currently dense sets always have an implicit base index of 0. If the
//   If the dominant pattern for using these will include sets with a real
//   lower bound much higher than 0, then updating the implementation with
//   an explicit base index (a multiple of 64) might make sense since. That
//   would allow sets to omit unused masks at the head, which could speed
//   up many of the operations on dense sets.
type dense struct {
	mask   []uint64       // mask with 1 bit/index in the range [beg, end)
	str    string         // (cached) result of String()
	cpuset *cpuset.CPUSet // (cached) result of CPUSet()
}

const (
	MASKBITS = 64 // MASKBITS bits fit into a single mask word.
)

// NewDenseSet creates a new dense index set with the given indices.
func NewDenseSet(indices ...int) IdxSet {
	return newDenseSet(indices...)
}

// ParseDenseSet parses the given string into a dense index set.
func ParseDenseSet(str string) (IdxSet, error) {
	s := &dense{}
	if err := s.Parse(str); err != nil {
		return nil, err
	}
	return s, nil
}

// MustParseDenseSet parses the given string into a dense index set.
func MustParseDenseSet(str string) IdxSet {
	s := &dense{}
	return s.MustParse(str)
}

func (s *dense) Clone() IdxSet {
	return s.clone()
}

func (s *dense) Reset() IdxSet {
	*s = dense{}
	return s
}

func (s *dense) Size() int {
	// Notes:
	//   github.com/tmthrgd/go-popcount implements CountSlice64() which
	//   could speed this up further. Recent golang compilers already
	//   generate POPCNT for math/bits.OnesCount64() so this should not
	//   be hopelessly slow (unless the set is large and not really dense).
	size := 0
	for _, m := range s.mask {
		size += bits.OnesCount64(m)
	}
	return size
}

func (s *dense) Indices() []int {
	indices := make([]int, 0, 64)
	s.ForEach(
		func(idx int) bool {
			indices = append(indices, idx)
			return false
		})
	return indices
}

func (s *dense) Add(indices ...int) IdxSet {
	altered := 0
	for _, idx := range indices {
		if idx < 0 {
			err := idxsetError("invalid (negative) index: %d", idx)
			panic(fmt.Sprintf("%v", err))
		}
		s.expand(idx)
		altered += s.setbit(idx)
	}
	s.altered(altered != 0)
	return s
}

func (s *dense) Del(indices ...int) IdxSet {
	altered := 0
	for _, idx := range indices {
		if idx < 0 {
			err := idxsetError("invalid (negative) index: %d", idx)
			panic(fmt.Sprintf("%v", err))
		}
		if s.spans(idx) {
			altered += s.clrbit(idx)
		}
	}
	s.altered(altered != 0)
	return s
}

func (s *dense) Contains(indices ...int) bool {
	for _, idx := range indices {
		if idx < 0 {
			err := idxsetError("invalid (negative) index: %d", idx)
			panic(fmt.Sprintf("%v", err))
		}
		if !s.spans(idx) || s.getbit(idx) == 0 {
			return false
		}
	}
	return true
}

func (s *dense) Union(other IdxSet) IdxSet {
	var r IdxSet

	if o, ok := other.(*dense); ok {
		r = s.union(o)
	} else {
		r = s.clone().Add(other.Indices()...).(*dense)
	}

	return r
}

func (s *dense) Unite(other IdxSet) IdxSet {
	if o, ok := other.(*dense); ok {
		return s.unite(o)
	} else {
		s.Add(other.Indices()...)
	}

	return s
}

func (s *dense) Intersection(other IdxSet) IdxSet {
	var r IdxSet

	if o, ok := other.(*dense); ok {
		r = s.intersection(o)
	} else {
		indices := []int{}
		for _, idx := range other.Indices() {
			if s.Contains(idx) {
				indices = append(indices, idx)
			}
		}
		r = newDenseSet(indices...)
	}

	return r
}

func (s *dense) Intersect(other IdxSet) IdxSet {
	if o, ok := other.(*dense); ok {
		s.intersect(o)
	} else {
		var indices []int
		for _, idx := range other.Indices() {
			if s.Contains(idx) {
				indices = append(indices, idx)
			}
		}
		*s = *newDenseSet(indices...)
	}

	return s
}

func (s *dense) Difference(other IdxSet) IdxSet {
	var r IdxSet

	if o, ok := other.(*dense); ok {
		r = s.difference(o)
	} else {
		r = s.clone()
		r.Del(other.Indices()...)
	}

	return r
}

func (s *dense) Subtract(other IdxSet) IdxSet {
	if o, ok := other.(*dense); ok {
		s.subtract(o)
	} else {
		s.Del(other.Indices()...)
	}

	return s
}

func (s *dense) Equals(other IdxSet) bool {
	var r bool

	if o, ok := other.(*dense); ok {
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

func (s *dense) String() string {
	if s.str == "" {
		s.str = toString(s)
	}
	return s.str
}

func (s *dense) Parse(str string) error {
	return parse(s, str)
}

func (s *dense) MustParse(str string) IdxSet {
	return mustParse(s, str)
}

func (s *dense) ForEach(fn func(int) bool) {
	base := 0
	for _, m := range s.mask {
		for m != 0 {
			idx := bits.TrailingZeros64(m)
			if fn(base + idx) {
				return
			}
			m &^= 1 << idx
		}
		base += MASKBITS
	}
}

func (s *dense) CPUSet() cpuset.CPUSet {
	if s.cpuset == nil {
		cset := cpuset.NewCPUSet(s.Indices()...)
		s.cpuset = &cset
	}
	return *s.cpuset
}

func newDenseSet(indices ...int) *dense {
	s := &dense{}
	s.Add(indices...)
	return s
}

func (s *dense) clone() *dense {
	return &dense{
		mask:   append([]uint64{}, s.mask...),
		str:    s.str,
		cpuset: s.cpuset,
	}
}

func (s *dense) union(o *dense) *dense {
	var min, max int
	var mask []uint64

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		min, max, mask = dl, ol, o.mask
	} else {
		min, max, mask = ol, dl, s.mask
	}

	r := &dense{mask: make([]uint64, max)}

	for i := 0; i < min; i++ {
		r.mask[i] = s.mask[i] | o.mask[i]
	}
	for i := min; i < max; i++ {
		r.mask[i] = mask[i]
	}

	return r
}

func (s *dense) unite(o *dense) *dense {
	var mask []uint64

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		mask = s.mask
		s.mask = make([]uint64, ol)
		copy(s.mask, o.mask)
	} else {
		mask = o.mask
	}

	for i, m := range mask {
		s.mask[i] |= m
	}

	s.altered(true)

	return s
}

func (s *dense) intersection(o *dense) *dense {
	var min int

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		min = dl
	} else {
		min = ol
	}

	r := &dense{mask: make([]uint64, min)}

	for i := 0; i < min; i++ {
		r.mask[i] = s.mask[i] & o.mask[i]
	}

	return r
}

func (s *dense) intersect(o *dense) *dense {
	var min int

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		min = dl
	} else {
		min = ol
	}

	for i := 0; i < min; i++ {
		s.mask[i] &= o.mask[i]
	}
	s.mask = s.mask[:min]

	s.altered(true)

	return s
}

func (s *dense) difference(o *dense) *dense {
	var min int
	var mask []uint64

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		min = dl
	} else {
		min = ol
		mask = s.mask[min:]
	}

	r := &dense{mask: make([]uint64, min)}

	for i := 0; i < min; i++ {
		r.mask[i] = s.mask[i] &^ o.mask[i]
	}
	if mask != nil {
		r.mask = append(r.mask, mask...)
	}

	return r
}

func (s *dense) subtract(o *dense) *dense {
	var min int

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		min = dl
	} else {
		min = ol
	}

	for i := 0; i < min; i++ {
		s.mask[i] &^= o.mask[i]
	}

	s.altered(true)

	return s
}

func (s *dense) equals(o *dense) bool {
	var min int
	var mask []uint64

	if dl, ol := len(s.mask), len(o.mask); dl < ol {
		min, mask = dl, o.mask
	} else {
		min, mask = ol, s.mask
	}

	for i := 0; i < min; i++ {
		if s.mask[i] != o.mask[i] {
			return false
		}
	}

	for _, m := range mask[min:] {
		if m != 0 {
			return false
		}
	}

	return true
}

func (s *dense) altered(altered bool) {
	if altered {
		s.str = ""
		s.cpuset = nil
	}
}

func (s *dense) spans(idx int) bool {
	return maskidx(idx) < len(s.mask)
}

func (s *dense) expand(idx int) {
	var count int

	if w, l := maskidx(idx), len(s.mask); w < l {
		return
	} else {
		count = 1 + (w - l)
	}

	s.mask = append(s.mask, make([]uint64, count)...)
}

func (s *dense) setbit(idx int) int {
	w, b := bitidx(idx)
	prev := (s.mask[w] & (1 << b)) >> b
	s.mask[w] |= 1 << b
	return int(prev) ^ 1
}

func (s *dense) clrbit(idx int) int {
	w, b := bitidx(idx)
	prev := (s.mask[w] & (1 << b)) >> b
	s.mask[w] &^= 1 << b
	return int(prev) ^ 0
}

func (s *dense) getbit(idx int) int {
	w, b := bitidx(idx)
	if (s.mask[w] & (1 << b)) != 0 {
		return 1
	}
	return 0
}

func bitidx(idx int) (int, int) {
	w := idx / MASKBITS
	b := idx & (MASKBITS - 1)
	return w, b
}

func maskidx(idx int) int {
	return idx / MASKBITS
}
