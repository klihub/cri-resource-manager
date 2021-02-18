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

package runtimes

import (
	"fmt"
	"path"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
)

const (
	// CriClass is the class name of the default runtime class.
	CriClass = "CRI"
	// KataClass is the class name of any Kata-container based runtime class.
	KataClass = "kata"
)

// classmap defines handler-class mapping for known runtime classes
type ClassMap struct {
	Classes []Class
}

// A single runtime-class declaration.
type Class struct {
	// Name of the class, used as Pod.GetRuntimeClass() for matching handlers
	Name string
	// HandlerPattern, used to path.Match()'ed against Pod.Config.RuntimeHandler
	HandlerPattern string
}

// our runtime classmap
var classMap = defaultClasses().(*ClassMap)

// MatchHandler searches a class matching the handler.
func MatchHandler(handler string) string {
	if handler == "" {
		return CriClass
	}
	for _, class := range classMap.Classes {
		if class.HandlerPattern == "" {
			return class.Name
		}
		if match, _ := path.Match(class.HandlerPattern, handler); match {
			return class.Name
		}
	}
	return ""
}

// defaultClasses
func defaultClasses() interface{} {
	return &ClassMap{
		Classes: []Class{
			{Name: KataClass, HandlerPattern: "kata*"},
			{Name: CriClass, HandlerPattern: ""},
		},
	}
}

// checkClasses checks classMap for bad patterns.
func checkClasses(event pkgcfg.Event, src pkgcfg.Source) error {
	for _, class := range classMap.Classes {
		if _, err := path.Match(class.HandlerPattern, "test"); err != nil {
			return fmt.Errorf("invalid handler pattern %q for class %q: %v",
				class.HandlerPattern, class.Name, err)
		}
	}
	return nil
}

func init() {
	pkgcfg.Register("runtime", "runtime handler/class mapping", classMap, defaultClasses,
		pkgcfg.WithNotify(checkClasses))
}
