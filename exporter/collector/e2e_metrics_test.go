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
	sendMetricsJsonToCollector(t, metricsJson)
	time.Sleep(time.Second * 5)

	t.Logf("Going to stop collector")
	shutdownCollector()
	t.Logf("Collector stopped")
}

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
