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
	"testing"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/collector/internal/e2ecollector"
	"github.com/stretchr/testify/require"
)

func TestE2eMetrics(t *testing.T) {
	t.Parallel()
	t.Logf("Going to start collector")
	shutdownCollector, err := e2ecollector.OtelColMain()
	require.NoError(t, err)
	t.Logf("Collector started")

	t.Logf("Going to stop collector")
	shutdownCollector()
	t.Logf("Collector stopped")
}
