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

package main

import (
	"fmt"
	"github.com/intel/cri-resource-manager/pkg/sysfs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var appID string
var sys sysfs.System

type status struct {
	// process name, allowed cpus and memory
	name string
	cpus string
	mems string
	// assigned shared, exclusive and isolated CPUs
	shared    string
	exclusive string
	isolated  string
	// quivalent NUMA nodes, sockets
	nodes   []int
	sockets []int
	// any errors encountered during discovery
	err error
}

func cpusToNodesAndSockets(cpustr string) (string, string) {
	cpus := cpuset.MustParse(cpustr)
	sockets := []string{}
	nodes := []string{}

	if sys == nil {
		unknown := "unknown (system discovery failed)"
		return unknown, unknown
	}

	for _, id := range sys.NodeIDs() {
		cset := sys.Node(id).CPUSet()
		if !cpus.Intersection(cset).IsEmpty() {
			nodes = append(nodes, strconv.FormatInt(int64(id), 10))
		}
	}

	for _, id := range sys.PackageIDs() {
		cset := sys.Package(id).CPUSet()
		if !cpus.Intersection(cset).IsEmpty() {
			sockets = append(sockets, strconv.FormatInt(int64(id), 10))
		}
	}

	return strings.Join(nodes, ","), strings.Join(sockets, ",")
}

func (s *status) Read() {
	s.err = sysfs.ParseFileEntries("/proc/self/status",
		map[string]interface{}{
			"Name:":              &s.name,
			"Cpus_allowed_list:": &s.cpus,
			"Mems_allowed_list:": &s.mems,
		},
		func(line string) (string, string, error) {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) < 1 {
				return "", "", nil
			}
			key := fields[0]
			val := strings.Join(fields[1:], " ")
			return key, val, nil
		},
	)

	if s.err != nil {
		return
	}

	s.err = sysfs.ParseFileEntries("/.cri-resmgr/resources.sh",
		map[string]interface{}{
			"SHARED_CPUS":    &s.shared,
			"EXCLUSIVE_CPUS": &s.exclusive,
			"ISOLATED_CPUS":  &s.isolated,
		},
		func(line string) (string, string, error) {
			if len(line) == 0 || line[0] == '#' {
				return "", "", nil
			}
			keyval := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(keyval) < 2 {
				return "", "", nil
			}

			key := keyval[0]
			val := strings.Trim(keyval[1], "\"")
			return key, val, nil
		},
	)
}

func (s *status) String() string {
	if s.err != nil {
		return fmt.Sprintf("failed to read status: %v", s.err)
	}

	nodes, sockets := cpusToNodesAndSockets(s.cpus)

	shared, exclusive, isolated := "", "", ""
	if s.shared != "" {
		shared = "    - shared: " + s.shared + "\n"
	}
	if s.exclusive != "" {
		exclusive = "    - exclusive: " + s.exclusive + "\n"
	}
	if s.isolated != "" {
		isolated = "    - isolated: " + s.isolated + "\n"
	}

	return fmt.Sprintf("%s status (%s):\n"+
		"  allowed:\n"+
		"    - cpus: %s\n"+
		"    - memory: %s\n"+
		"  assigned cpus:\n"+
		"%s"+
		"%s"+
		"%s"+
		"  equivalent topology placement:\n"+
		"    - assigned CPU sockets: %s\n"+
		"    - assigned NUMA nodes: %s\n",
		s.name, appID,
		s.cpus, s.mems,
		shared, exclusive, isolated,
		sockets, nodes)
}

func handler(w http.ResponseWriter, r *http.Request) {
	status := &status{}
	status.Read()
	fmt.Fprintf(w, "%s", status.String())
}

func main() {
	if len(os.Args) < 2 {
		appID = "unknown"
	} else {
		appID = os.Args[1]
	}
	s, err := sysfs.DiscoverSystem()
	if err != nil {
		fmt.Printf("Warning: system discovery failed: %v\n", err)
	} else {
		sys = s
	}

	port := "8080"
	if len(os.Args) == 3 {
		port = os.Args[2]
	}

	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
