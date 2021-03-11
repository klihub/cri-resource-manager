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
	"math/rand"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

func TestCreate(t *testing.T) {
	type testCase struct {
		str      string
		members  []int
		chkStr   string
		chkSlice []int
	}

	T := func(o interface{}) testCase {
		switch value := o.(type) {
		case string:
			chk := cpuset.MustParse(value)
			return testCase{
				str:      value,
				chkStr:   chk.String(),
				chkSlice: chk.ToSlice(),
			}
		case []int:
			chk := cpuset.NewCPUSet(value...)
			return testCase{
				members:  value,
				chkStr:   chk.String(),
				chkSlice: chk.ToSlice(),
			}
		default:
			testError(t, "invalid initializer type (%T) for test case", value)
			return testCase{str: "invalid initializer"}
		}
	}

	testCases := []testCase{
		T(""),
		T("0"), T("1"), T("3"), T("63"), T("64"), T("127"), T("128"),
		T("0-1"), T("0-2"), T("0-10"), T("0-63"), T("10-70"),
		T("0-64"), T("0-65"), T("63,64"), T("0-128"), T("127-257"),
		T("63-64,127-128,256-259,1023-1025"),
		T("0-50"), T("0-128"), T("64-72"),
		T("0,2,4,6,8,10,64,66,68,70"), T("1,3,5,7,9,11,65,67,69,71"),
		T("0-128,256-512"),

		T([]int{}),
		T([]int{0}), T([]int{1}), T([]int{3}), T([]int{63}), T([]int{64}),
		T([]int{127}), T([]int{128}),
		T([]int{0, 1}), T([]int{0, 1, 2}), T([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}),
		T([]int{60, 61, 62, 63, 64, 65, 66}), T([]int{126, 127, 128, 128, 255, 256, 257}),
		T([]int{63, 64, 127, 128, 256, 257, 258, 259, 1023, 1024, 1025}),
		T([]int{0, 2, 4, 6, 8, 10, 64, 66, 68, 70}), T([]int{1, 3, 5, 7, 9, 11, 65, 67, 69, 71}),
	}

	for _, kind := range []string{"dense", "sparse"} {
		for _, tc := range testCases {
			var (
				from    interface{}
				set     IdxSet
				str     string
				indices []int
			)
			if tc.members == nil {
				from = tc.str
			} else {
				from = tc.members
			}
			set = mkset(kind, from)
			str = set.String()
			indices = set.Indices()

			if str != tc.chkStr {
				testError(t, "%s: expected String() %q, got %q",
					kind, tc.chkStr, str)
			}
			if !reflect.DeepEqual(indices, tc.chkSlice) {
				testError(t, "%s: expected Indices() %v, got %v",
					kind, tc.chkSlice, indices)
			}
		}
	}

	getFuzzSet(t)
}

type operation int

const (
	union operation = iota
	intersection
	difference
	unite
	intersect
	subtract
)

func (o operation) String() string {
	switch o {
	case union:
		return "|"
	case intersection:
		return "&"
	case difference:
		return "-"
	case unite:
		return "|="
	case intersect:
		return "&="
	case subtract:
		return "-="
	}
	return "UNKNOWN-OP"
}

func TestSetOps(t *testing.T) {
	isetUnion := func(s1, s2 IdxSet) IdxSet {
		return s1.Union(s2)
	}
	isetIntersection := func(s1, s2 IdxSet) IdxSet {
		return s1.Intersection(s2)
	}
	isetDifference := func(s1, s2 IdxSet) IdxSet {
		return s1.Difference(s2)
	}
	isetUnite := func(s1, s2 IdxSet) IdxSet {
		return s1.Clone().Unite(s2)
	}
	isetIntersect := func(s1, s2 IdxSet) IdxSet {
		return s1.Clone().Intersect(s2)
	}
	isetSubtract := func(s1, s2 IdxSet) IdxSet {
		return s1.Clone().Subtract(s2)
	}
	csetUnion := func(s1, s2 cpuset.CPUSet) cpuset.CPUSet {
		return s1.Union(s2)
	}
	csetIntersection := func(s1, s2 cpuset.CPUSet) cpuset.CPUSet {
		return s1.Intersection(s2)
	}
	csetDifference := func(s1, s2 cpuset.CPUSet) cpuset.CPUSet {
		return s1.Difference(s2)
	}
	isetOps := map[operation]func(IdxSet, IdxSet) IdxSet{
		union:        isetUnion,
		intersection: isetIntersection,
		difference:   isetDifference,
		unite:        isetUnite,
		intersect:    isetIntersect,
		subtract:     isetSubtract,
	}
	csetOps := map[operation]func(cpuset.CPUSet, cpuset.CPUSet) cpuset.CPUSet{
		union:        csetUnion,
		intersection: csetIntersection,
		difference:   csetDifference,
		unite:        csetUnion,
		intersect:    csetIntersection,
		subtract:     csetDifference,
	}

	allOps := []operation{
		union, intersection, difference,
		unite, intersect, subtract,
	}

	type testCase struct {
		set1 []IdxSet
		set2 []IdxSet
		chk  []string
	}

	T := func(set1, set2 string) testCase {
		tc := testCase{
			set1: []IdxSet{MustParseDenseSet(set1), MustParseSparseSet(set1)},
			set2: []IdxSet{MustParseDenseSet(set2), MustParseSparseSet(set2)},
			chk:  make([]string, 0, len(allOps)),
		}
		s1 := cpuset.MustParse(set1)
		s2 := cpuset.MustParse(set2)
		for _, op := range allOps {
			tc.chk = append(tc.chk, csetOps[op](s1, s2).String())
		}
		return tc
	}

	f := getFuzzSet(t)
	F := func() testCase {
		idx1, idx2 := rand.Intn(len(f)), rand.Intn(len(f))
		f1 := f[idx1]
		f2 := f[idx2]
		tc := testCase{
			set1: []IdxSet{f1.set[0], f1.set[1]},
			set2: []IdxSet{f2.set[0], f2.set[1]},
			chk:  make([]string, 0, len(allOps)),
		}
		s1 := cpuset.MustParse(f1.str)
		s2 := cpuset.MustParse(f2.str)
		for _, op := range allOps {
			tc.chk = append(tc.chk, csetOps[op](s1, s2).String())
		}
		return tc
	}

	testCases := []testCase{
		T("", ""),
		T("0", "0"),
		T("0", "1"),
		T("0-1", "1-2"),
		T("0-5", "3-7"),
		T("0-31", "31-69"),
		T("0-63", "0-31"),
		T("0-64", "0-64"),
		T("31,32,63,64,127,128", "31,63,127"),
	}

	for i := 0; i < 4096; i++ {
		testCases = append(testCases, F())
	}

	for _, tc := range testCases {
		for combo := 0; combo < 2*2; combo++ {
			set1 := tc.set1[combo&0x1]
			set2 := tc.set2[(combo&0x2)>>1]

			for i, op := range allOps {
				res := isetOps[op](set1, set2)
				chk := tc.chk[i]

				if res.String() != chk {
					testError(t, "expected %q %s %q == %q, got %q",
						set1.String(), op.String(), set2.String(), chk, res.String())
					testError(t, "IdxSet types were %T, %T", set1, set2)
				} else {
					//fmt.Printf("%s %s %s = %s: OK\n",
					//	set1.String(), op.String(), set2.String(), res.String())
				}
			}
		}
	}
}

func mkset(kind string, from interface{}) IdxSet {
	switch kind {
	case "dense":
		switch value := from.(type) {
		case string:
			return MustParseDenseSet(value)
		case []int:
			return NewDenseSet(value...)
		}
	case "sparse":
		switch value := from.(type) {
		case string:
			return MustParseSparseSet(value)
		case []int:
			return NewSparseSet(value...)
		}
	}

	panic(fmt.Sprintf("invalid IdxSet kind %q", kind))
}

type refString string
type refSlice []int

func (s refString) String() string {
	return cpuset.MustParse(string(s)).String()
}

func (s refString) Indices() []int {
	return cpuset.MustParse(string(s)).ToSlice()
}

func (s refSlice) String() string {
	return cpuset.NewCPUSet([]int(s)...).String()
}

func (s refSlice) Indices() []int {
	return cpuset.NewCPUSet([]int(s)...).ToSlice()
}

func testError(t *testing.T, format string, args ...interface{}) {
	t.Errorf(format, args...)
}

type set struct {
	str     string
	indices []int
	set     []IdxSet
}

var fuzz []*set
var once sync.Once

func getFuzzSet(t *testing.T) []*set {
	once.Do(func() {
		slice := []*set{}

		sweepRange := 512
		for _, size := range []int{0, 2, 4, 8, 16, 32, 64, 128} {
			for beg := 0; beg < sweepRange; beg++ {
				str := strconv.Itoa(beg)
				if size > 0 {
					str += "-" + strconv.Itoa(beg+size)
				}
				chk := cpuset.MustParse(str)

				f := &set{
					str:     chk.String(),
					indices: chk.ToSlice(),
					set: []IdxSet{
						MustParseDenseSet(str),
						MustParseSparseSet(str),
					},
				}

				failed := false
				if i := f.set[0].Indices(); !reflect.DeepEqual(i, f.indices) {
					t.Errorf("incorrect dense %q indices: %v, expect %v\n",
						f.str, i, f.indices)
					failed = true
				}
				if i := f.set[1].Indices(); !reflect.DeepEqual(i, f.indices) {
					t.Errorf("incorrect sparse %q indices: %v, expect %v\n",
						f.str, i, f.indices)
					failed = true
				}

				if s := f.set[0].String(); s != f.str {
					t.Errorf("incorrect dense %q str: %q\n",
						f.str, f.set[0].String())
					failed = true
				}
				if s := f.set[1].String(); s != f.str {
					t.Errorf("incorrect sparse %q str: %q\n",
						f.str, f.set[1].String())
					failed = true
				}

				if !f.set[0].Equals(f.set[0]) {
					t.Errorf("%q: dense/dense equality failed\n", f.str)
					failed = true
				}
				if !f.set[0].Equals(f.set[1]) {
					t.Errorf("%q: dense/sparse equality failed\n", f.str)
					failed = true
				}
				if !f.set[1].Equals(f.set[0]) {
					t.Errorf("%q: dense/sparse equality failed\n", f.str)
					failed = true
				}
				if !f.set[1].Equals(f.set[1]) {
					t.Errorf("%q: sparse/sparse equality failed\n", f.str)
					failed = true
				}

				if !failed {
					slice = append(slice, f)
				}
			}
		}

		fuzzRange := 4 * 64
		for beg := 0; beg < fuzzRange; beg += 13 {
			for _, everyNth := range []int{1, 2, 3, 5, 7} {
				for end := 0; end < fuzzRange; end++ {
					members := []int{}
					for i := beg; i < end; i++ {
						if (i % everyNth) == 0 {
							members = append(members, i)
						}
					}

					chk := cpuset.NewCPUSet(members...)

					f := &set{
						str:     chk.String(),
						indices: chk.ToSlice(),
						set: []IdxSet{
							NewDenseSet(members...),
							NewSparseSet(members...),
						},
					}

					failed := false
					if i := f.set[0].Indices(); !reflect.DeepEqual(i, f.indices) {
						t.Errorf("incorrect dense %q indices: %v, expect %v\n",
							f.str, i, f.indices)
						failed = true
					}
					if i := f.set[1].Indices(); !reflect.DeepEqual(i, f.indices) {
						t.Errorf("incorrect sparse %q indices: %v, expect %v\n",
							f.str, i, f.indices)
						failed = true
					}

					if s := f.set[0].String(); s != f.str {
						t.Errorf("incorrect dense %q str: %q\n",
							f.str, f.set[0].String())
						failed = true
					}
					if s := f.set[1].String(); s != f.str {
						t.Errorf("incorrect sparse %q str: %q\n",
							f.str, f.set[1].String())
						failed = true
					}

					if !f.set[0].Equals(f.set[0]) {
						t.Errorf("%q: dense/dense equality failed\n", f.str)
						failed = true
					}
					if !f.set[0].Equals(f.set[1]) {
						t.Errorf("%q: dense/sparse equality failed\n", f.str)
						failed = true
					}
					if !f.set[1].Equals(f.set[0]) {
						t.Errorf("%q: dense/sparse equality failed\n", f.str)
						failed = true
					}
					if !f.set[1].Equals(f.set[1]) {
						t.Errorf("%q: sparse/sparse equality failed\n", f.str)
						failed = true
					}

					if !failed {
						slice = append(slice, f)
					}
				}
			}
		}

		rand.Shuffle(len(slice), func(i, j int) {
			slice[i], slice[j] = slice[j], slice[i]
		})

		fuzz = slice
	})

	return fuzz
}
