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

package cri

import (
	"fmt"
	"os"
	"path"

	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"

	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// KataController is the name and runtime class of this controller.
	KataController = cache.Kata
)

// katactl encapsulated the runtime state of our Kata enforcement/controller.
type katactl struct {
	cache cache.Cache
}

// Our logger instance.
var log logger.Logger = logger.NewLogger(KataController)

// Our Kata controller singleton instance.
var singleton *katactl

// getKataController returns our singleton Kata controller instance.
func getKataController() control.Controller {
	if singleton == nil {
		singleton = &katactl{}
	}
	return singleton
}

// Start initializes the controller for enforcing decisions.
func (ctl *katactl) Start(cache cache.Cache, client client.Client) error {
	ctl.cache = cache
	return nil
}

// Stop shuts down the controller.
func (ctl *katactl) Stop() {
}

// PreCreateHook is the Kata controller pre-create hook.
func (ctl *katactl) PreCreateHook(c cache.Container) error {
	return nil
}

// PreStartHook is the Kata controller pre-start hook.
func (ctl *katactl) PreStartHook(c cache.Container) error {
	if c.GetRuntimeClass() != KataController {
		return nil
	}

	if err := ctl.updateResources(c); err != nil {
		return err
	}

	c.ClearPending(KataController)
	return nil
}

// PostStartHook is the Kata controller post-start hook.
func (ctl *katactl) PostStartHook(c cache.Container) error {
	return nil
}

// PostUpdateHook is the Kata controller post-update hook.
func (ctl *katactl) PostUpdateHook(c cache.Container) error {
	if c.GetRuntimeClass() != KataController {
		return nil
	}

	if err := ctl.updateResources(c); err != nil {
		return err
	}

	c.ClearPending(KataController)
	return nil
}

// PostStop is the Kata controller post-stop hook.
func (ctl *katactl) PostStopHook(c cache.Container) error {
	return nil
}

// updateResources updates the resources for this kata container.
func (ctl *katactl) updateResources(c cache.Container) error {
	resources := c.GetLinuxResources()
	if resources == nil {
		return nil
	}

	pod, ok := c.GetPod()
	if !ok {
		return nil
	}

	cgroup := "/vc/kata_" + pod.GetID()

	if v := resources.CpuShares; v != 0 {
		entry := path.Join("/sys/fs/cgroup/cpu", cgroup, "cpu.shares")
		if err := writeCgroupEntry(entry, v); err != nil {
			return err
		}
	}
	if v := resources.CpuPeriod; v != 0 {
		entry := path.Join("/sys/fs/cgroup/cpu", cgroup, "cpu.cfs_period_us")
		if err := writeCgroupEntry(entry, v); err != nil {
			return err
		}
	}
	if v := resources.CpuQuota; v != 0 {
		entry := path.Join("/sys/fs/cgroup/cpu", cgroup, "cpu.cfs_quota_us")
		if err := writeCgroupEntry(entry, v); err != nil {
			return err
		}
	}
	if v := resources.CpusetCpus; v != "" {
		entry := path.Join("/sys/fs/cgroup/cpuset", cgroup, "cpuset.cpus")
		if err := writeCgroupEntry(entry, v); err != nil {
			return err
		}
	}
	if v := resources.CpusetMems; v != "" {
		entry := path.Join("/sys/fs/cgroup/cpuset", cgroup, "cpuset.mems")
		if err := writeCgroupEntry(entry, v); err != nil {
			return err
		}
	}

	return nil
}

// writeCgroupEntry writes an value to a cgroup entry.
func writeCgroupEntry(entry string, v interface{}) error {
	value := fmt.Sprintf("%v\n", v)

	log.Info("*** writing %v to %q", v, entry)

	f, err := os.OpenFile(entry, os.O_WRONLY, 0644)
	if err != nil {
		return criError("%q: failed to write %v: %v", entry, v, err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(value)); err != nil {
		return criError("%q: failed to write %v: %v", entry, v, err)
	}
	return nil
}

// criError creates an CRI-controller-specific formatted error message.
func criError(format string, args ...interface{}) error {
	return fmt.Errorf("cri: "+format, args...)
}

// Register us as a controller.
func init() {
	control.Register(KataController, "Kata controller", getKataController())
}
