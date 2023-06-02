// Copyright 2022 Google LLC
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

package collector

import (
	"fmt"
	"regexp"
	"strings"

	starlarkproto "go.starlark.net/lib/proto"
	"go.starlark.net/starlark"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv "go.opentelemetry.io/collector/semconv/v1.18.0"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/proto"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping"
)

type attributes struct {
	Attrs pcommon.Map
}

func (attrs *attributes) GetString(key string) (string, bool) {
	value, ok := attrs.Attrs.Get(key)
	if ok {
		return value.AsString(), ok
	}
	return "", false
}

// defaultResourceToMonitoredResource pdata Resource to a GCM Monitored Resource.
func defaultResourceToMonitoredResource(resource pcommon.Resource) *monitoredrespb.MonitoredResource {
	attrs := resource.Attributes()
	gmr := resourcemapping.ResourceAttributesToMonitoredResource(&attributes{
		Attrs: attrs,
	})
	newLabels := make(labels, len(gmr.Labels))
	for k, v := range gmr.Labels {
		newLabels[k] = sanitizeUTF8(v)
	}
	mr := &monitoredrespb.MonitoredResource{
		Type:   gmr.Type,
		Labels: newLabels,
	}
	return mr
}

// makeStarlarkResourceToMonitoredResource returns a resource mapping function that transforms
// the resource using the provided starlark expression
func makeStarlarkResourceToMonitoredResource(
	starlarkSrc string,
	log *zap.Logger,
) (func(resource pcommon.Resource) *monitoredrespb.MonitoredResource, error) {
	thread := &starlark.Thread{}
	// proto.SetPool(thread, protoregistry.GlobalFiles)
	predeclared := starlark.StringDict{
		// "proto": proto.Module,
		"MonitoredResource": starlark.NewBuiltin(
			"MonitoredResource",
			func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				var mrType starlark.String
				var mrLabels *starlark.Dict
				err := starlark.UnpackArgs(fn.Name(), args, kwargs, "type", &mrType, "labels", &mrLabels)
				if err != nil {
					return nil, err
				}

				labelsAsMap := map[string]string{}
				for _, k := range mrLabels.Keys() {
					keyStr, ok := k.(starlark.String)
					if !ok {
						return nil, fmt.Errorf("labels must be a dict of str keys and values")
					}
					value, _, err := mrLabels.Get(keyStr)
					if err != nil {
						return nil, err
					}
					valueStr, ok := value.(starlark.String)
					if !ok {
						return nil, fmt.Errorf("labels must be a dict of str keys and values")
					}
					labelsAsMap[k.(starlark.String).GoString()] = valueStr.GoString()
				}
				mr := &monitoredrespb.MonitoredResource{
					Type:   mrType.GoString(),
					Labels: labelsAsMap,
				}
				bytes, err := proto.Marshal(mr)
				if err != nil {
					return nil, err
				}
				return starlarkproto.Unmarshal((&monitoredrespb.MonitoredResource{}).ProtoReflect().Descriptor(), bytes)
			},
		),
	}

	// TODO starlark.ExprFunc may be better here
	globals, err := starlark.ExecFile(thread, "map_resource.star", starlarkSrc, predeclared)
	if err != nil {
		return nil, err
	}

	// Get the function
	starlarkFn := globals["map_resource"]
	if starlarkFn == nil {
		return nil, fmt.Errorf("Starlark code did not declare a function named map_resource")
	}

	return func(resource pcommon.Resource) *monitoredrespb.MonitoredResource {
			dict := starlark.NewDict(resource.Attributes().Len())
			resource.Attributes().Range(func(k string, v pcommon.Value) bool {
				dict.SetKey(starlark.String(k), starlark.String(v.AsString()))
				return true
			})
			res, err := starlark.Call(thread, starlarkFn, starlark.Tuple{dict}, nil)
			if err != nil {
				log.Panic("Got an error while evaluating starlark code", zap.Error(err))
			}
			resMsg, ok := res.(*starlarkproto.Message)
			if !ok {
				log.Sugar().Panicf("Error evaluating starlark map_resource function: return value was not proto Message, got %v", res)
			}
			log.Sugar().Info("Got message returned: %+v", resMsg)
			resMr, ok := resMsg.Message().(*monitoredrespb.MonitoredResource)
			if !ok {
				log.Sugar().Panicf("Error evaluating starlark map_resource function: return value was the wrong type of protobuf message, got %v", resMsg)
			}
			return resMr
		},
		nil
}

// resourceToLabels converts the Resource attributes into labels.
// TODO(@damemi): Refactor to pass control-coupling lint check.
//
//nolint:revive
func resourceToLabels(
	resource pcommon.Resource,
	serviceResourceLabels bool,
	resourceFilters []ResourceFilter,
	log *zap.Logger,
) labels {
	attrs := pcommon.NewMap()
	resource.Attributes().Range(func(k string, v pcommon.Value) bool {
		// Is a service attribute and should be included
		if serviceResourceLabels &&
			(k == semconv.AttributeServiceName ||
				k == semconv.AttributeServiceNamespace ||
				k == semconv.AttributeServiceInstanceID) {
			if len(v.AsString()) > 0 {
				v.CopyTo(attrs.PutEmpty(k))
			}
			return true
		}
		// Matches one of the resource filters
		for _, resourceFilter := range resourceFilters {
			if match, _ := regexp.Match(resourceFilter.Regex, []byte(k)); strings.HasPrefix(k, resourceFilter.Prefix) && match {
				v.CopyTo(attrs.PutEmpty(k))
				return true
			}
		}
		return true
	})
	return attributesToLabels(attrs)
}
