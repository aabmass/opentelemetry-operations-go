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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/collector/internal/e2ecollector"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2eMetrics(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	t.Logf("Going to start collector")
	shutdownCollector, err := e2ecollector.OtelColMain()
	require.NoError(t, err)
	t.Logf("Collector started")

	endTime := time.Now()
	startTime := endTime.Add(-time.Second)

	metricsJsonBytes := loadOTLPMetricsFixture(t, "testdata/metrics-fixture.json", startTime, endTime)
	sendMetricsJsonToCollector(t, metricsJsonBytes)
	time.Sleep(time.Second * 5)

	t.Logf("Going to stop collector")
	shutdownCollector()
	t.Logf("Collector stopped")

	assertMetrics(ctx, t, assertMetricsParams{
		StartTime: startTime, EndTime: endTime, MetricName: "e2e.request_count",
	})
}

type assertMetricsParams struct {
	StartTime  time.Time
	EndTime    time.Time
	MetricName string
}

func assertMetrics(ctx context.Context, t *testing.T, params assertMetricsParams) {
	client, err := monitoring.NewMetricClient(ctx)
	require.NoError(t, err)
	defer client.Close()

	expectedGcmRes := &monitoringpb.ListTimeSeriesResponse{}
	loadExpectationFixture(t, "testdata/gcm-expectation-fixture.json", expectedGcmRes)
	for _, ts := range expectedGcmRes.GetTimeSeries() {
		for _, point := range ts.GetPoints() {
			point.GetInterval().StartTime = timestamppb.New(params.StartTime)
			point.GetInterval().EndTime = timestamppb.New(params.EndTime)
		}
	}

	it := client.ListTimeSeries(ctx, &monitoringpb.ListTimeSeriesRequest{
		Name: fmt.Sprintf("projects/%v", os.Getenv("PROJECT_ID")),
		Filter: fmt.Sprintf(
			`metric.type = "custom.googleapis.com/opencensus/%v"`, params.MetricName,
		),
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(params.StartTime.Add(-time.Second)),
			EndTime:   timestamppb.New(params.EndTime.Add(time.Second)),
		},
		PageSize: 100,
	})
	_, err = it.Next()
	require.NoError(t, err)
	res, ok := it.Response.(*monitoringpb.ListTimeSeriesResponse)
	require.True(t, ok)

	// jsonBytes, err := protojson.MarshalOptions{Indent: "  "}.Marshal(res)
	// require.NoError(t, err)
	// err = ioutil.WriteFile("testdata/gcm-expectation-fixture.json", jsonBytes, 0644)
	// require.NoError(t, err)

	diff := cmp.Diff(expectedGcmRes, res, protocmp.Transform(), timestamppbApproxEqualOpt)
	assert.Emptyf(t, diff, "Expected GCM response and actual GCM response differ:\n%v", diff)
}

func loadOTLPMetricsFixture(t *testing.T, fixturePath string, startTime time.Time, endTime time.Time) []byte {
	bytes, err := ioutil.ReadFile(fixturePath)
	require.NoError(t, err)
	metricsMap := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(bytes, &metricsMap))

	// Update timestamps from the fixture
	for _, rm := range metricsMap["resourceMetrics"].([]interface{}) {
		for _, ilm := range rm.(map[string]interface{})["instrumentationLibraryMetrics"].([]interface{}) {
			for _, m := range ilm.(map[string]interface{})["metrics"].([]interface{}) {
				mMap := m.(map[string]interface{})

				// Coalesce with other potential data aggregations like histogram, gauge, ...
				aggregation := mMap["sum"]

				for _, dp := range aggregation.(map[string]interface{})["dataPoints"].([]interface{}) {
					dp := dp.(map[string]interface{})
					dp["startTimeUnixNano"] = startTime.UnixNano()
					dp["timeUnixNano"] = endTime.UnixNano()
				}
			}
		}
	}

	require.NoError(t, err)
	updatedBytes, err := json.Marshal(metricsMap)
	require.NoError(t, err)
	return updatedBytes
}

func loadExpectationFixture(t *testing.T, fixturePath string, loadInto proto.Message) {
	bytes, err := ioutil.ReadFile(fixturePath)
	require.NoError(t, err)
	require.NoError(t, protojson.Unmarshal(bytes, loadInto))
}

func sendMetricsJsonToCollector(t *testing.T, json []byte) {
	res, err := http.Post("http://localhost:4318/v1/metrics", "application/json", bytes.NewReader(json))
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

// cmp.Option that compares if timestamps are equal within 1ms. Needed because
// golang's protobuf JSON serialization only preserve's timestamps to the
// nearest µs.
var timestamppbApproxEqualOpt cmp.Option = protocmp.FilterMessage(
	&timestamppb.Timestamp{}, cmp.Comparer(func(x, y protocmp.Message) bool {
		xTime := time.Unix(x["seconds"].(int64), int64(x["nanos"].(int32)))
		yTime := time.Unix(y["seconds"].(int64), int64(y["nanos"].(int32)))
		delta := xTime.Sub(yTime)
		if delta < 0 {
			delta *= -1
		}
		return delta.Milliseconds() <= 1
	}),
)
