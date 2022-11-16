// Copyright The OpenTelemetry Authors
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

package serialization // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/dynatraceexporter/internal/serialization"

import (
	"fmt"
	"time"

	"github.com/dynatrace-oss/dynatrace-metric-utils-go/metric/dimensions"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/ttlmap"
)

type Serializer struct {
	logger          *zap.Logger
	throttledLogger *zap.Logger
}

func CreateSerializer(logger *zap.Logger) *Serializer {
	return &Serializer{logger: logger, throttledLogger: createSampledLogger(logger)}
}

func (s *Serializer) SerializeMetric(prefix string, metric pmetric.Metric, defaultDimensions, staticDimensions dimensions.NormalizedDimensionList, prev *ttlmap.TTLMap) ([]string, error) {
	var metricLines []string

	ce := s.logger.Check(zap.DebugLevel, "SerializeMetric")
	var points int

	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		metricLines = s.serializeGauge(prefix, metric, defaultDimensions, staticDimensions, metricLines)
	case pmetric.MetricTypeSum:
		metricLines = s.serializeSum(prefix, metric, defaultDimensions, staticDimensions, prev, metricLines)
	case pmetric.MetricTypeHistogram:
		metricLines = s.serializeHistogram(prefix, metric, defaultDimensions, staticDimensions, metricLines)
	default:
		return nil, fmt.Errorf("metric type %s unsupported", metric.Type().String())
	}

	if ce != nil {
		ce.Write(zap.String("DataType", metric.Type().String()), zap.Int("points", points))
	}

	return metricLines, nil
}

func (s *Serializer) makeCombinedDimensions(defaultDimensions dimensions.NormalizedDimensionList, dataPointAttributes pcommon.Map, staticDimensions dimensions.NormalizedDimensionList) dimensions.NormalizedDimensionList {
	dimsFromAttributes := make([]dimensions.Dimension, 0, dataPointAttributes.Len())

	dataPointAttributes.Range(func(k string, v pcommon.Value) bool {
		if v.Type() != pcommon.ValueTypeStr {
			s.throttledLogger.Info(
				"unexpected non-string attribute value. converting to string",
				zap.String("key", k),
				zap.String("type", v.Type().String()),
			)
		}

		dimsFromAttributes = append(dimsFromAttributes, dimensions.NewDimension(k, v.AsString()))
		return true
	})
	return dimensions.MergeLists(
		defaultDimensions,
		dimensions.NewNormalizedDimensionList(dimsFromAttributes...),
		staticDimensions,
	)
}

// createSampledLogger was copied from https://github.com/open-telemetry/opentelemetry-collector/blob/v0.26.0/exporter/exporterhelper/queued_retry.go#L108
func createSampledLogger(logger *zap.Logger) *zap.Logger {
	if logger.Core().Enabled(zapcore.DebugLevel) {
		// Debugging is enabled. Don't do any sampling.
		return logger
	}

	// Create a logger that samples all messages to 1 per 60 seconds initially,
	// and 1/1000 of messages after that.
	opts := zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(
			core,
			time.Minute,
			1,
			1000,
		)
	})
	return logger.WithOptions(opts)
}
