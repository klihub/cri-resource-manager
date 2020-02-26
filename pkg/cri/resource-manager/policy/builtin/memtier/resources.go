// Copyright 2019 Intel Corporation. All Rights Reserved.
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

package memtier

import (
	"fmt"

	"github.com/intel/cri-resource-manager/pkg/cpuallocator"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

// Supply represents avaialbe CPU and memory capacity of a node.
type Supply interface {
	// GetNode returns the node supplying this capacity.
	GetNode() Node
	// Clone creates a copy of this Supply.
	Clone() Supply
	// IsolatedCPUs returns the isolated cpuset in this supply.
	IsolatedCPUs() cpuset.CPUSet
	// SharableCPUs returns the sharable cpuset in this supply.
	SharableCPUs() cpuset.CPUSet
	// Granted returns the locally granted capacity in this supply.
	Granted() int
	// Cumulate cumulates the given supply into this one.
	Cumulate(Supply)
	// AccountAllocate accounts for (removes) allocated exclusive capacity from the supply.
	AccountAllocate(Grant)
	// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
	AccountRelease(Grant)
	// GetScore calculates how well this supply fits/fulfills the given request.
	GetScore(Request) Score
	// Allocate allocates CPU capacity from this supply and returns it as a grant.
	Allocate(Request) (Grant, error)
	// Release releases a previously allocated grant.
	Release(Grant)
	// String returns a printable representation of this supply.
	String() string
}

// Request represents CPU and memory resources requested by a container.
type Request interface {
	// GetContainer returns the container requesting CPU capacity.
	GetContainer() cache.Container
	// String returns a printable representation of this request.
	String() string

	// FullCPUs return the number of full CPUs requested.
	FullCPUs() int
	// CPUFraction returns the amount of fractional milli-CPU requested.
	CPUFraction() int
	// Isolate returns whether isolated CPUs are preferred for this request.
	Isolate() bool
	// Elevate returns the requested elevation/allocation displacement for this request.
	Elevate() int
	// MemoryType returns the type(s) of requested memory.
	MemoryType() memoryType

	GetMemLimit() uint64
}

// Grant represents CPU and memory capacity allocated to a container from a node.
type Grant interface {
	// GetContainer returns the container CPU capacity is granted to.
	GetContainer() cache.Container
	// GetNode returns the node that granted CPU capacity to the container.
	GetNode() Node
	// ExclusiveCPUs returns the exclusively granted non-isolated cpuset.
	ExclusiveCPUs() cpuset.CPUSet
	// SharedCPUs returns the shared granted cpuset.
	SharedCPUs() cpuset.CPUSet
	// SharedPortion returns the amount of CPUs in milli-CPU granted.
	SharedPortion() int
	// IsolatedCpus returns the exclusively granted isolated cpuset.
	IsolatedCPUs() cpuset.CPUSet
	// MemoryType returns the type(s) of granted memory.
	MemoryType() memoryType
	// String returns a printable representation of this grant.
	String() string
}

// Score represents how well a supply can satisfy a request.
type Score interface {
	// Calculate the actual score from the collected parameters.
	Eval() float64
	// Supply returns the supply associated with this score.
	Supply() Supply
	// Request returns the request associated with this score.
	Request() Request

	IsolatedCapacity() int
	SharedCapacity() int
	Colocated() int
	HintScores() map[string]float64

	String() string
}

// supply implements our Supply interface.
type supply struct {
	node     Node          // node supplying CPUs
	isolated cpuset.CPUSet // isolated CPUs at this node
	sharable cpuset.CPUSet // sharable CPUs at this node
	granted  int           // amount of sharable allocated
	normMem  uint64        // available normal memory at this node
	slowMem  uint64        // available slow memory at this node
	fastMem  uint64        // available fast memory at this node
}

var _ Supply = &supply{}

// request implements our CpuRequest interface.
type request struct {
	container cache.Container // container for this request
	full      int             // number of full CPUs requested
	fraction  int             // amount of fractional CPU requested
	isolate   bool            // prefer isolated exclusive CPUs

	memReq  uint64     // memory request
	memLim  uint64     // memory limit
	memType memoryType // requested types of memory

	// elevate indicates how much to elevate the actual allocation of the
	// container in the tree of pools. Or in other words how many levels to
	// go up in the tree starting at the best fitting pool, before assigning
	// the container to an actual pool. Currently ignored.
	elevate int
}

var _ Request = &request{}

// grant implements our CpuGrant interface.
type grant struct {
	container cache.Container // container CPU is granted to
	node      Node            // node CPU is supplied from
	exclusive cpuset.CPUSet   // exclusive CPUs
	portion   int             // milliCPUs granted from shared set
	memType   memoryType      // requested types of memmory
}

var _ Grant = &grant{}

// score implements our Score interface.
type score struct {
	supply    Supply             // CPU supply (node)
	req       Request            // CPU request (container)
	isolated  int                // remaining isolated CPUs
	shared    int                // remaining shared capacity
	colocated int                // number of colocated containers
	hints     map[string]float64 // hint scores
}

var _ Score = &score{}

// newSupply creates CPU supply for the given node, cpusets and existing grant.
func newSupply(n Node, isolated, sharable cpuset.CPUSet, granted int) Supply {
	return &supply{
		node:     n,
		isolated: isolated.Clone(),
		sharable: sharable.Clone(),
		granted:  granted,
	}
}

// GetNode returns the node supplying CPU.
func (cs *supply) GetNode() Node {
	return cs.node
}

// Clone clones the given CPU supply.
func (cs *supply) Clone() Supply {
	return newSupply(cs.node, cs.isolated, cs.sharable, cs.granted)
}

// IsolatedCpus returns the isolated CPUSet of this supply.
func (cs *supply) IsolatedCPUs() cpuset.CPUSet {
	return cs.isolated.Clone()
}

// SharableCpus returns the sharable CPUSet of this supply.
func (cs *supply) SharableCPUs() cpuset.CPUSet {
	return cs.sharable.Clone()
}

// Granted returns the locally granted sharable CPU capacity.
func (cs *supply) Granted() int {
	return cs.granted
}

// Cumulate more CPU to supply.
func (cs *supply) Cumulate(more Supply) {
	mcs := more.(*supply)

	cs.isolated = cs.isolated.Union(mcs.isolated)
	cs.sharable = cs.sharable.Union(mcs.sharable)
	cs.granted += mcs.granted

	cs.normMem += mcs.normMem
	cs.slowMem += mcs.slowMem
	cs.fastMem += mcs.fastMem
}

// AccountAllocate accounts for (removes) allocated exclusive capacity from the supply.
func (cs *supply) AccountAllocate(g Grant) {
	if cs.node.IsSameNode(g.GetNode()) {
		return
	}
	exclusive := g.ExclusiveCPUs()
	cs.isolated = cs.isolated.Difference(exclusive)
	cs.sharable = cs.sharable.Difference(exclusive)
}

// AccountRelease accounts for (reinserts) released exclusive capacity into the supply.
func (cs *supply) AccountRelease(g Grant) {
	if cs.node.IsSameNode(g.GetNode()) {
		return
	}

	ncs := cs.node.GetSupply()
	nodecpus := ncs.IsolatedCPUs().Union(ncs.SharableCPUs())
	grantcpus := g.ExclusiveCPUs().Intersection(nodecpus)

	isolated := grantcpus.Intersection(ncs.IsolatedCPUs())
	sharable := grantcpus.Intersection(ncs.SharableCPUs())
	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
}

// Allocate allocates a grant from the supply.
func (cs *supply) Allocate(r Request) (Grant, error) {
	var exclusive cpuset.CPUSet
	var err error

	cr := r.(*request)

	// allocate isolated exclusive CPUs or slice them off the sharable set
	switch {
	case cr.full > 0 && cs.isolated.Size() >= cr.full:
		exclusive, err = takeCPUs(&cs.isolated, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"can't allocate %d exclusive CPUs from %s of %s",
				cr.full, cs.isolated, cs.node.Name())
		}

	case cr.full > 0 && (1000*cs.sharable.Size()-cs.granted)/1000 > cr.full:
		exclusive, err = takeCPUs(&cs.sharable, nil, cr.full)
		if err != nil {
			return nil, policyError("internal error: "+
				"can't slice %d exclusive CPUs from %s(-%d) of %s",
				cr.full, cs.sharable, cs.granted, cs.node.Name())
		}
	}

	// allocate requested portion of the sharable set
	if cr.fraction > 0 {
		if 1000*cs.sharable.Size()-cs.granted < cr.fraction {
			return nil, policyError("internal error: "+
				"not enough sharable CPU for %d in %s(-%d) of %s",
				cr.fraction, cs.sharable, cs.granted, cs.node.Name())
		}
		cs.granted += cr.fraction
	}

	grant := newGrant(cs.node, cr.GetContainer(), exclusive, cr.fraction, cr.memType)

	cs.node.DepthFirst(func(n Node) error {
		n.FreeSupply().AccountAllocate(grant)
		return nil
	})

	return grant, nil
}

// Release returns CPU from the given grant to the supply.
func (cs *supply) Release(g Grant) {
	isolated := g.ExclusiveCPUs().Intersection(cs.node.GetSupply().IsolatedCPUs())
	sharable := g.ExclusiveCPUs().Difference(isolated)

	cs.isolated = cs.isolated.Union(isolated)
	cs.sharable = cs.sharable.Union(sharable)
	cs.granted -= g.SharedPortion()

	cs.node.DepthFirst(func(n Node) error {
		n.FreeSupply().AccountRelease(g)
		return nil
	})
}

// String returns the CPU supply as a string.
func (cs *supply) String() string {
	none, isolated, sharable, sep := "-", "", "", ""

	if !cs.isolated.IsEmpty() {
		isolated = fmt.Sprintf("isolated:%s", cs.isolated)
		sep = ", "
		none = ""
	}
	if !cs.sharable.IsEmpty() {
		sharable = fmt.Sprintf("%ssharable:%s (granted:%d, free: %d)", sep,
			cs.sharable, cs.granted, 1000*cs.sharable.Size()-cs.granted)
		none = ""
	}

	return "<" + cs.node.Name() + " CPU: " + none + isolated + sharable + ">"
}

// newRequest creates a new CPU request for the given container.
func newRequest(container cache.Container) Request {
	pod, _ := container.GetPod()
	full, fraction, isolate, elevate := cpuAllocationPreferences(pod, container)

	req, lim, mtype := memoryAllocationPreference(pod, container)

	return &request{
		container: container,
		full:      full,
		fraction:  fraction,
		isolate:   isolate,
		memReq:    req,
		memLim:    lim,
		memType:   mtype,
		elevate:   elevate,
	}
}

// GetContainer returns the container requesting CPU.
func (cr *request) GetContainer() cache.Container {
	return cr.container
}

// String returns aprintable representation of the CPU request.
func (cr *request) String() string {
	isolated := map[bool]string{false: "", true: "isolated "}[cr.isolate]
	switch {
	case cr.full == 0 && cr.fraction == 0:
		return fmt.Sprintf("<CPU request " + cr.container.PrettyName() + ": ->")

	case cr.full > 0 && cr.fraction > 0:
		return fmt.Sprintf("<CPU request "+cr.container.PrettyName()+": "+
			"%sfull: %d, shared: %d>", isolated, cr.full, cr.fraction)

	case cr.full > 0:
		return fmt.Sprintf("<CPU request "+
			cr.container.PrettyName()+": %sfull: %d>", isolated, cr.full)

	default:
		return fmt.Sprintf("<CPU request "+
			cr.container.PrettyName()+": shared: %d>", cr.fraction)
	}
}

// FullCPUs return the number of full CPUs requested.
func (cr *request) FullCPUs() int {
	return cr.full
}

// CPUFraction returns the amount of fractional milli-CPU requested.
func (cr *request) CPUFraction() int {
	return cr.fraction
}

// Isolate returns whether isolated CPUs are preferred for this request.
func (cr *request) Isolate() bool {
	return cr.isolate
}

// Elevate returns the requested elevation/allocation displacement for this request.
func (cr *request) Elevate() int {
	return cr.elevate
}

func (cr *request) GetMemLimit() uint64 {
	return cr.memLim
}

func (cr *request) GetMemType() memoryType {
	return cr.memType
}

// MemoryType returns the requested type of memory for the grant.
func (cr *request) MemoryType() memoryType {
	if cr.memType == memoryUnspec {
		return defaultMemoryType
	}
	return cr.memType
}

// Score collects data for scoring this supply wrt. the given request.
func (cs *supply) GetScore(req Request) Score {
	score := &score{
		supply: cs,
		req:    req,
	}

	cr := req.(*request)
	full, part := cr.full, cr.fraction
	if full == 0 && part == 0 {
		part = 1
	}

	// calculate free shared capacity
	score.shared = 1000*cs.sharable.Size() - cs.node.GrantedSharedCPU()

	// calculate isolated node capacity CPU
	if cr.isolate {
		score.isolated = cs.isolated.Size() - full
	}

	// if we don't want isolated or there is not enough, calculate slicable capacity
	if !cr.isolate || score.isolated < 0 {
		score.shared -= 1000 * full
	}

	// calculate fractional capacity
	score.shared -= part

	// calculate colocation score
	for _, grant := range cs.node.Policy().allocations.CPU {
		if grant.GetNode().NodeID() == cs.node.NodeID() {
			score.colocated++
		}
	}

	// calculate real hint scores
	hints := cr.container.GetTopologyHints()
	score.hints = make(map[string]float64, len(hints))

	for provider, hint := range cr.container.GetTopologyHints() {
		log.Debug(" - evaluating topology hint %s", hint)
		score.hints[provider] = cs.node.HintScore(hint)
	}

	// calculate any fake hint scores
	pod, _ := cr.container.GetPod()
	key := pod.GetName() + ":" + cr.container.GetName()
	if fakeHints, ok := opt.FakeHints[key]; ok {
		for provider, hint := range fakeHints {
			log.Debug(" - evaluating fake hint %s", hint)
			score.hints[provider] = cs.node.HintScore(hint)
		}
	}
	if fakeHints, ok := opt.FakeHints[cr.container.GetName()]; ok {
		for provider, hint := range fakeHints {
			log.Debug(" - evaluating fake hint %s", hint)
			score.hints[provider] = cs.node.HintScore(hint)
		}
	}

	return score
}

// Eval...
func (score *score) Eval() float64 {
	return 1.0
}

func (score *score) Supply() Supply {
	return score.supply
}

func (score *score) Request() Request {
	return score.req
}

func (score *score) IsolatedCapacity() int {
	return score.isolated
}

func (score *score) SharedCapacity() int {
	return score.shared
}

func (score *score) Colocated() int {
	return score.colocated
}

func (score *score) HintScores() map[string]float64 {
	return score.hints
}

func (score *score) String() string {
	return fmt.Sprintf("<CPU score: node %s, isolated:%d, shared:%d, colocated:%d, hints: %v>",
		score.supply.GetNode().Name(), score.isolated, score.shared, score.colocated, score.hints)
}

// newGrant creates a CPU grant from the given node for the container.
func newGrant(n Node, c cache.Container, exclusive cpuset.CPUSet, portion int, mt memoryType) Grant {
	return &grant{
		node:      n,
		container: c,
		exclusive: exclusive,
		portion:   portion,
		memType:   mt,
	}
}

// GetContainer returns the container this grant is valid for.
func (cg *grant) GetContainer() cache.Container {
	return cg.container
}

// GetNode returns the Node this grant is allocated to.
func (cg *grant) GetNode() Node {
	return cg.node
}

// ExclusiveCPUs returns the non-isolated exclusive CPUSet in this grant.
func (cg *grant) ExclusiveCPUs() cpuset.CPUSet {
	return cg.exclusive
}

// SharedCPUs returns the shared CPUSet in this grant.
func (cg *grant) SharedCPUs() cpuset.CPUSet {
	return cg.node.GetSupply().SharableCPUs()
}

// SharedPortion returns the milli-CPU allocation for the shared CPUSet in this grant.
func (cg *grant) SharedPortion() int {
	return cg.portion
}

// ExclusiveCPUs returns the isolated exclusive CPUSet in this grant.
func (cg *grant) IsolatedCPUs() cpuset.CPUSet {
	return cg.node.GetSupply().IsolatedCPUs().Intersection(cg.exclusive)
}

// MemoryType returns the requested type of memory for the grant.
func (cg *grant) MemoryType() memoryType {
	if cg.memType == memoryUnspec {
		return defaultMemoryType
	}
	return cg.memType
}

// String returns a printable representation of the CPU grant.
func (cg *grant) String() string {
	var isolated, exclusive, shared, sep string

	isol := cg.IsolatedCPUs()
	if !isol.IsEmpty() {
		isolated = fmt.Sprintf("isolated: %s", isol)
		sep = ", "
	}
	if !cg.exclusive.IsEmpty() {
		exclusive = fmt.Sprintf("%sexclusive: %s", sep, cg.exclusive)
		sep = ", "
	}
	if cg.portion > 0 {
		shared = fmt.Sprintf("%sshared: %s (%d milli-CPU)", sep,
			cg.node.FreeSupply().SharableCPUs(), cg.portion)
	}

	return fmt.Sprintf("<CPU grant for %s from %s: %s%s%s>",
		cg.container.PrettyName(), cg.node.Name(), isolated, exclusive, shared)
}

// takeCPUs takes up to cnt CPUs from a given CPU set to another.
func takeCPUs(from, to *cpuset.CPUSet, cnt int) (cpuset.CPUSet, error) {
	cset, err := cpuallocator.AllocateCpus(from, cnt, false)
	if err != nil {
		return cset, err
	}

	if to != nil {
		*to = to.Union(cset)
	}

	return cset, err
}
