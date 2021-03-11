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
	"strconv"
	"strings"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

//
// IdxSet defines the interface implemented by an unordered set of unique
// integer indices or IDs, or as generally referred to here an index set.
// IdxSet defines binary set operations for union, intersection, difference,
// and equality test. Additional functions are provided for parsing and
// stringification compatible with Linux CPU and memory set notation.
type IdxSet interface {
	// Clone creates a full copy of the set.
	Clone() IdxSet
	// Reset resets the set to be empty.
	Reset() IdxSet
	// Size returns the number of indices in the set.
	Size() int
	// Indices returns the sorted slice of indices in the set.
	Indices() []int
	// Add adds the given indices to the set.
	Add(...int) IdxSet
	// Del removes the given indices from the set.
	Del(...int) IdxSet
	// Contains returns true if all the given indices are in the set.
	Contains(...int) bool
	// Union returns the union of this and the given set.
	Union(IdxSet) IdxSet
	// Intersection returns the intersection of this and the given set.
	Intersection(IdxSet) IdxSet
	// Difference returns the difference of this and the given set.
	Difference(IdxSet) IdxSet
	// Unite is the mutating version of Union, updating the set.
	Unite(IdxSet) IdxSet
	// Intersect is the mutating version of Intersection, updating the set.
	Intersect(IdxSet) IdxSet
	// Subtract is the mutating version of Difference, updating the set.
	Subtract(IdxSet) IdxSet
	// Equals returns true if this and the given set are equal.
	Equals(IdxSet) bool
	// String returns a string representation of this set.
	String() string
	// Parse updates set to have the indices present in the string.
	Parse(string) error
	// MustParse calls Parse() and panics on errors.
	MustParse(string) IdxSet
	// ForEach calls the given function for each index in the set. The
	// function can stop further iteration by returning true.
	ForEach(func(int) bool)
	// CPUSet returns the index set as a CPUSet.
	CPUSet() cpuset.CPUSet
}

func toString(s IdxSet) string {
	str, beg, end := strings.Builder{}, -1, -1

	writeRange := func(beg, end int) {
		if beg < 0 {
			return
		}
		if str.Len() > 0 {
			str.WriteString(",")
		}
		str.WriteString(strconv.Itoa(beg))
		if end < 0 {
			return
		}
		str.WriteString("-")
		str.WriteString(strconv.Itoa(end))
	}

	s.ForEach(
		func(idx int) bool {
			switch {
			case beg < 0:
				beg = idx
			case end < 0 && idx == beg+1:
				end = idx
			case idx == end+1:
				end = idx
			default:
				writeRange(beg, end)
				beg = idx
				end = -1
			}
			return false
		})
	writeRange(beg, end)

	return str.String()
}

func parse(s IdxSet, str string) error {
	indices := []int{}
	if str != "" {
		for _, idxOrRange := range strings.Split(str, ",") {
			if rng := strings.SplitN(idxOrRange, "-", 2); len(rng) == 2 {
				beg, err := strconv.Atoi(rng[0])
				if err != nil {
					return idxsetError("invalid range %q in %q: %v",
						idxOrRange, str, err)
				}
				end, err := strconv.Atoi(rng[1])
				if err != nil {
					return idxsetError("invalid range %q in %q: %v",
						idxOrRange, str, err)
				}
				if beg > end {
					return idxsetError("invalid range %q in %q",
						idxOrRange, str)
				}
				for i := beg; i <= end; i++ {
					indices = append(indices, i)
				}
			} else {
				idx, err := strconv.Atoi(idxOrRange)
				if err != nil {
					return idxsetError("invalid index %q in %q: %v",
						idxOrRange, str, err)
				}
				indices = append(indices, idx)
			}
		}
	}

	s.Reset()
	s.Add(indices...)

	return nil
}

func mustParse(s IdxSet, str string) IdxSet {
	if err := parse(s, str); err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	return s
}

func idxsetError(format string, args ...interface{}) error {
	return fmt.Errorf("idxset error: "+format, args...)
}
