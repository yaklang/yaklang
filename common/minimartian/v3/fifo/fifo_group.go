// Copyright 2015 Google Inc. All rights reserved.
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

// Package fifo provides Group, which is a list of modifiers that are executed
// consecutively. By default, when an error is returned by a modifier, the
// execution of the modifiers is halted, and the error is returned. Optionally,
// when errror aggregation is enabled (by calling SetAggretateErrors(true)), modifier
// execution is not halted, and errors are aggretated and returned after all
// modifiers have been executed.
package fifo

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/v3"
)

// Group is a martian.RequestResponseModifier that maintains lists of
// request and response modifiers executed on a first-in, first-out basis.
type Group struct {
	reqmu   sync.RWMutex
	reqmods []martian.RequestModifier

	resmu   sync.RWMutex
	resmods []martian.ResponseModifier

	aggregateErrors bool
}

type groupJSON struct {
	Modifiers       []json.RawMessage `json:"modifiers"`
	AggregateErrors bool              `json:"aggregateErrors"`
}

// NewGroup returns a modifier group.
func NewGroup() *Group {
	return &Group{}
}

// SetAggregateErrors sets the error behavior for the Group. When true, the Group will
// continue to execute consecutive modifiers when a modifier in the group encounters an
// error. The Group will then return all errors returned by each modifier after all
// modifiers have been executed.  When false, if an error is returned by a modifier, the
// error is returned by ModifyRequest/Response and no further modifiers are run.
// By default, error aggregation is disabled.
func (g *Group) SetAggregateErrors(aggerr bool) {
	g.aggregateErrors = aggerr
}

// AddRequestModifier adds a RequestModifier to the group's list of request modifiers.
func (g *Group) AddRequestModifier(reqmod martian.RequestModifier) {
	g.reqmu.Lock()
	defer g.reqmu.Unlock()

	g.reqmods = append(g.reqmods, reqmod)
}

// AddResponseModifier adds a ResponseModifier to the group's list of response modifiers.
func (g *Group) AddResponseModifier(resmod martian.ResponseModifier) {
	g.resmu.Lock()
	defer g.resmu.Unlock()

	g.resmods = append(g.resmods, resmod)
}

// ModifyRequest modifies the request. By default, aggregateErrors is false; if an error is
// returned by a RequestModifier the error is returned and no further modifiers are run. When
// aggregateErrors is set to true, the errors returned by each modifier in the group are
// aggregated.
func (g *Group) ModifyRequest(req *http.Request) error {
	log.Debugf("fifo.ModifyRequest: %s", req.URL)
	g.reqmu.RLock()
	defer g.reqmu.RUnlock()

	merr := martian.NewMultiError()

	for _, reqmod := range g.reqmods {
		if err := reqmod.ModifyRequest(req); err != nil {
			if g.aggregateErrors {
				merr.Add(err)
				continue
			}

			return err
		}
	}

	if merr.Empty() {
		return nil
	}

	return merr
}

// ModifyResponse modifies the request. By default, aggregateErrors is false; if an error is
// returned by a RequestModifier the error is returned and no further modifiers are run. When
// aggregateErrors is set to true, the errors returned by each modifier in the group are
// aggregated.
func (g *Group) ModifyResponse(res *http.Response) error {
	if res == nil {
		return utils.Error("no response should be modified")
	}
	requ := ""
	if res.Request != nil {
		requ = res.Request.URL.String()
		log.Debugf("fifo.ModifyResponse: %s", requ)
	}
	g.resmu.RLock()
	defer g.resmu.RUnlock()

	merr := martian.NewMultiError()

	for _, resmod := range g.resmods {
		if err := resmod.ModifyResponse(res); err != nil {
			if g.aggregateErrors {
				merr.Add(err)
				continue
			}

			return err
		}
	}

	if merr.Empty() {
		return nil
	}

	return merr
}
