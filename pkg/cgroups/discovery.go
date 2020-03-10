// Copyright 2020 Intel Corporation. All Rights Reserved.
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

package cgroups

import (
	"io/ioutil"
	"strings"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

// Controller enumerates known cgroup controller types.
type ControllerType int

const (
	// BlkIO is the blkio cgroup controller
	BlkIO ControllerType = iota
	// CPU is the cpu cgroup controller
	CPU
	// CPUAcct is the cpuacct cgroup controller
	CPUAcct
	// CPUSet is the cpuset cgroup controller
	CPUSet
	// Devices is the devices cgroup controller
	Devices
	// Freezer is the freezer cgroup controller
	Freezer
	// HugeTLB is the hugetlb cgroup controller
	HugeTLB
	// NetCLS is the net_cls cgroup controller
	NetCLS
	// NetPrio is the net_prio cgroup controller
	NetPrio
	// Memory is the memory cgroup controller
	Memory
	// PerfEvent is the perf_event cgroup controller
	PerfEvent
	// PIDS is the pids cgroup controller
	PIDs
	// RDMA is the rdma cgroup controller
	RDMA
	// Systemd is the cgroup directory used by systemd
	Systemd
	// Cgroup2 is the unified mount path for cgroup v2.
	Cgroup2

	// mtab is the path for reading mount points.
	mtab = "/proc/mounts"
)

// ControllerTypeNames defines the names of known cgroup controller types.
var ControllerNames = map[ControllerType]string{
	BlkIO:     "blkio",
	CPU:       "cpu",
	CPUAcct:   "cpuacct",
	CPUSet:    "cpuset",
	Devices:   "devices",
	Freezer:   "freezer",
	HugeTLB:   "hugetlb",
	Memory:    "memory",
	NetCLS:    "net_cls",
	NetPrio:   "net_prio",
	PerfEvent: "perf_event",
	PIDs:      "pids",
	RDMA:      "rdma",
	Systemd:   "name=systemd",
	Cgroup2:   "cgroup2",
}

// ControllerTypes defines known controller types by name.
var ControllerTypes = map[string]ControllerType{
	ControllerNames[BlkIO]:     BlkIO,
	ControllerNames[CPUAcct]:   CPUAcct,
	ControllerNames[CPUSet]:    CPUSet,
	ControllerNames[Devices]:   Devices,
	ControllerNames[Freezer]:   Freezer,
	ControllerNames[HugeTLB]:   HugeTLB,
	ControllerNames[Memory]:    Memory,
	ControllerNames[NetCLS]:    NetCLS,
	ControllerNames[NetPrio]:   NetPrio,
	ControllerNames[PerfEvent]: PerfEvent,
	ControllerNames[PIDs]:      PIDs,
	ControllerNames[RDMA]:      RDMA,
	ControllerNames[Systemd]:   Systemd,
	ControllerNames[Cgroup2]:   Cgroup2,
}

// cgroup mount points for various controllers.
var mounts = map[ControllerType]string{}

// our logger instance
var log = logger.Default()

// GetControllerDir returns the mount path for the given controller.
func GetControllerDir(t ControllerType) string {
	if len(mounts) == 0 {
		detectControllers()
	}
	return mounts[t]
}

// detectControllers reads /proc/mounts to find out the mount points of controller.
func detectControllers() {
	buf, err := ioutil.ReadFile(mtab)
	if err != nil {
		log.Warn("failed to read %s: %v", mtab, err)
		return
	}

	paths := map[ControllerType]string{}
	for _, entry := range strings.Split(string(buf), "\n") {
		split := strings.Split(entry, " ")
		if len(split) < 4 {
			continue
		}

		_, path, fstype, optstr := split[0], split[1], split[2], split[3]

		switch fstype {
		case "cgroup2":
			mounts[Cgroup2] = path
		case "cgroup":
			break
		default:
			continue
		}

		options := strings.Split(optstr, ",")
		for i := len(options) - 1; i >= 0; i-- {
			if ctype, ok := ControllerTypes[options[i]]; ok {
				mounts[ctype] = path
				break
			}
		}
	}
	mounts = paths
}

func init() {
	detectControllers()
	for ctype, path := range mounts {
		log.Info("* cgroup path for %d (%s): '%s'", ctype, ControllerNames[ctype], path)
	}
}
