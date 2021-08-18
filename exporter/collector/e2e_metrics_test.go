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

package googlecloudexporter_test

import (
	"io"
	"net/http"
	"path"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/collector/internal/e2ecollector"
	"github.com/stretchr/testify/require"
)

type metricsVars struct {
	StartTimeUnixNano int64
	TimeUnixNano      int64
}

// // type metricTransform func(metrics pdata.Metrics)

// type transform struct {
// 	// path to thing to update
// 	path []string
// 	// apply function returns the updated value. data parameters is the current
// 	// value being updated.
// 	apply func(data interface{}) interface{}
// }

// func applyTransform(data map[string]interface{}, transform transform) error {
// 	level := []map[string]interface{}{data}

// 	for _, p := range transform.path[:len(transform.path)-1] {
// 		nextLevel :=
// 		for _, obj  := range level {

// 		}

// 		nextPath := func(element map[string]interface{}) {
// 			nextCur, success := cur[p].(map[string]interface{})
// 			if !success {
// 				keys := []string{}
// 				for k := range cur {
// 					keys = append(keys, k)
// 				}
// 				return fmt.Errorf(
// 					`field at part "%v" is not compatible with map[string]interface{} (got %T). `+
// 						`valid keys were: %v`, p, cur[p], keys,
// 				)
// 			}
// 			cur = nextCur
// 		}
// 	}

// 	lastKey := transform.path[len(transform.path)-1]
// 	cur[lastKey] = transform.apply(cur[lastKey])
// 	return nil
// }
// func applyTransforms(data map[string]interface{}, transforms ...transform) error {
// 	for _, tr := range transforms {
// 		err := applyTransform(data, tr)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func makeUpdateTimestampsTransforms(startTime time.Time, endTime time.Time) []transform {
// 	basePath := []string{
// 		"resourceMetrics", "instrumentationLibraryMetrics", "metrics",
// 	}
// 	startTimePath := []string{
// 		"dataPoints", "startTimeUnixNano",
// 	}
// 	timePath := []string{
// 		"dataPoints", "timeUnixNano",
// 	}

// 	return []transform{
// 		{
// 			path: append(append(basePath, "sum"), startTimePath...),
// 			apply: func(interface{}) interface{} {
// 				return fmt.Sprint(startTime.UnixNano())
// 			},
// 		},
// 		{
// 			path: append(append(basePath, "sum"), timePath...),
// 			apply: func(interface{}) interface{} {
// 				return fmt.Sprint(endTime.UnixNano())
// 			},
// 		},
// 	}
// }

// var metricTransforms = []metricTransform{
// 	func(metrics pdata.Metrics) {
// 		for i := 0; i < metrics.ResourceMetrics().Len(); i++ {
// 			rm := metrics.ResourceMetrics().At(i)
// 			for j := 0; j < rm.InstrumentationLibraryMetrics().Len(); j++ {
// 				ilm := rm.InstrumentationLibraryMetrics().At(i)
// 				for k := 0; k < ilm.Metrics().Len(); k++ {
// 					m := ilm.Metrics().At(i)
// 					m.Summary()
// 					m.Gauge()
// 				}
// 			}
// 		}
// 	},
// }

func TestE2eMetrics(t *testing.T) {
	t.Parallel()
	t.Logf("Going to start collector")
	shutdownCollector, err := e2ecollector.OtelColMain()
	require.NoError(t, err)
	t.Logf("Collector started")

	endTime := time.Now()
	startTime := endTime.Add(-time.Second)

	metricsJson := loadFixture(t, "testdata/metrics-fixture.json.tmpl", metricsVars{
		StartTimeUnixNano: startTime.UnixNano(),
		TimeUnixNano:      endTime.UnixNano(),
	})
	// metrics := loadFixtureJson(t, "testdata/metrics-fixture.json")
	// t.Logf("Before: %v", metrics)
	// err = applyTransforms(metrics, makeUpdateTimestampsTransforms(startTime, endTime)...)
	// require.NoError(t, err)
	// t.Logf("After: %v", metrics)
	sendMetricsJsonToCollector(t, metricsJson)
	time.Sleep(time.Second * 5)

	t.Logf("Going to stop collector")
	shutdownCollector()
	t.Logf("Collector stopped")
}

// func loadFixtureJson(t *testing.T, fixturePath string) map[string]interface{} {
// 	bytes, err := ioutil.ReadFile(fixturePath)
// 	require.NoError(t, err)
// 	var data map[string]interface{}
// 	err = json.Unmarshal(bytes, &data)
// 	require.NoError(t, err)
// 	return data
// }

func loadFixture(t *testing.T, fixturePath string, data interface{}) string {
	baseName := path.Base(fixturePath)
	tmpl := template.Must(template.New(baseName).ParseFiles(fixturePath))
	builder := strings.Builder{}
	err := tmpl.Execute(&builder, data)
	require.NoError(t, err)
	return builder.String()
}

func sendMetricsJsonToCollector(t *testing.T, json string) {
	res, err := http.Post("http://localhost:4318/v1/metrics", "application/json", strings.NewReader(json))
	require.NoError(t, err)
	defer res.Body.Close()
	bytes, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.EqualValuesf(
		t,
		200,
		res.StatusCode,
		`Excepted 200 response from OTLP HTTP receiver, got %v. Response body: "%v"`,
		res.StatusCode,
		string(bytes),
	)
}
