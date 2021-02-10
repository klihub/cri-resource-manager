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
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	v1 "k8s.io/api/core/v1"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// Cpuset is the name and mount path of the cpuset controller.
	Cpuset = "cpuset"
	// RootDir is the sysfs cgroup parent directory path.
	RootDir = "/sys/fs/cgroup"
	// CpusetDir is the mount path for the v1 cpuset controller.
	CpusetDir = RootDir + "/cpuset"
)

var (
	// Root is the common sysfs directory for mounting cgroup controllers.
	Root = "/sys/fs/cgroup"
	// V2path is the mount point for the cgroup V2 pseudofilesystem.
	V2path string
	// KubeletRoot is the --cgroup-root passed to kubelet.
	KubeletRoot string
	// our logger instance
	pathlog = logger.NewLogger("cgroups")

	// pod path generating functions
	pathGenFns = []func(UID, class string) []string{
		// cgroups driver
		func(UID, class string) []string {
			UID = "pod" + UID
			if class[0] == 'g' {
				class = ""
			}
			return []string{
				// with --cgroups-per-qos
				path.Join(CpusetDir, KubeletRoot, "kubepods", class, UID),
				// without --cgroups-per-qos
				path.Join(CpusetDir, KubeletRoot, "kubepods", UID),
			}
		},
		// systemd driver
		func(UID, class string) []string {
			UID = strings.ReplaceAll(UID, "-", "_")
			if class[0] == 'g' {
				UID = "kubepods-" + UID + ".slice"
				class = ""
			} else {
				UID = "kubepods-" + class + "-pod" + UID + ".slice"
				class = "kubepods-" + class + ".slice"
			}
			kubelet := KubeletRoot
			if kubelet != "" {
				kubelet += ".slice"
			}
			return []string{
				// with --cgroups-per-qos
				path.Join(CpusetDir, kubelet, "kubepods.slice", class, UID),
				// without --cgroups-per-qos
				path.Join(CpusetDir, kubelet, "kubepods.slice", UID),
			}
		},
	}
)

// FindPodCgroupParent brute-force searches for a pod cgroup parent dir.
func FindPodCgroupParent(UID string, qos v1.PodQOSClass) (string, v1.PodQOSClass) {
	var classes []v1.PodQOSClass
	var cgpaths []string

	if qos != "" {
		classes = []v1.PodQOSClass{
			qos,
			v1.PodQOSGuaranteed,
			v1.PodQOSBestEffort,
			v1.PodQOSBurstable,
		}
		cgpaths = []string{
			strings.ToLower(string(qos)),
			"guaranteed",
			"besteffort",
			"burstable",
		}
	} else {
		classes = []v1.PodQOSClass{
			v1.PodQOSGuaranteed,
			v1.PodQOSBestEffort,
			v1.PodQOSBurstable,
		}
		cgpaths = []string{
			"guaranteed",
			"besteffort",
			"burstable",
		}
	}

	for classIdx, class := range cgpaths {
		for fnIdx, fn := range pathGenFns {
			for _, podPath := range fn(UID, class) {
				if info, err := os.Stat(podPath); err == nil {
					if info.Mode().IsDir() {
						qos = classes[classIdx]
						podPath = strings.TrimPrefix(podPath, CpusetDir)

						// Bring the correct function to the front. If we only
						// have a single runtime class, this will work for all
						// future lookups.
						if fnIdx != 0 {
							pathGenFns[fnIdx] = pathGenFns[0]
							pathGenFns[0] = fn
						}

						return podPath, qos
					}
				}
			}
		}
	}

	return "", v1.PodQOSClass("")
}

// FindContainerCgroupDir brute-force searches for a container cgroup dir.
func FindContainerCgroupDir(podCgroupDir, podID, ID string) string {
	if podCgroupDir == "" {
		return ""
	}

	dirs := []string{
		path.Join(CpusetDir, podCgroupDir, "cri-containerd-"+ID+".scope"),
		path.Join(CpusetDir, podCgroupDir, ID),
		path.Join(CpusetDir, podCgroupDir, "crio-"+ID+".scope"),
		path.Join(CpusetDir, "vc", "kata_"+"podID"),
		path.Join(CpusetDir, podCgroupDir, "kata_"+podID),
	}

	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil {
			if info.Mode().IsDir() {
				return strings.TrimPrefix(dir, CpusetDir)
			}
		}
	}

	return ""
}

// GetProcesses reads cgroup.procs entry from the cgroup directory.
func GetProcesses(dir string) ([]string, error) {
	return readTaskFile(path.Join(CpusetDir, dir, "cgroup.procs"))
}

// GetProcesses reads tasks entry from the cgroup directory.
func GetTasks(dir string) ([]string, error) {
	return readTaskFile(path.Join(CpusetDir, dir, "tasks"))
}

// readTaskFile reads entries from cgroup tasks or cgroup.procs.
func readTaskFile(file string) ([]string, error) {
	var pids []string

	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open cgroup entry %s: %v", file, err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		pids = append(pids, s.Text())
	}

	return pids, nil
}

func init() {
	flag.StringVar(&V2path, "cgroupv2-path", "/sys/fs/cgroup/unified",
		"Path to cgroup-v2 mountpoint")
}
