// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2ecollector

import (
	"fmt"

	"go.opentelemetry.io/collector/config/configparser"
	"go.opentelemetry.io/collector/service"
)

const (
	confPath = "testdata/config-e2e.yaml"
)

type shutdownFunc func()

type parserProvider struct{}

func (p *parserProvider) Get() (*configparser.Parser, error) {
	cp, err := configparser.NewParserFromFile(confPath)
	if err != nil {
		return nil, fmt.Errorf("error loading config file %q: %v", confPath, err)
	}
	return cp, nil
}

// OtelColMain starts the collector in a separate goroutine and returns when it
// is fully initialized. It returns a function to shutdown the collector,
// blocking until it is completely shutdown.
func OtelColMain() (shutdownFunc, error) {
	factories, err := components()
	if err != nil {
		return nil, err
	}

	app, err := service.New(service.CollectorSettings{
		Factories:      factories,
		ParserProvider: &parserProvider{},
	})
	if err != nil {
		return nil, err
	}

	go func() {
		err := app.Run()
		if err != nil {
			panic(err)
		}
	}()

	for {
		state := <-app.GetStateChannel()

		if state < service.Running {
			// wait again until Running
		} else if state == service.Running {
			return makeShutdownFunc(app), nil
		} else {
			return nil, fmt.Errorf("collector never entered Running state, got %v", state)
		}
	}
}

func makeShutdownFunc(app *service.Collector) shutdownFunc {
	return func() {
		app.Shutdown()

		for {
			state := <-app.GetStateChannel()
			if state == service.Closed {
				return
			}
		}
	}
}
