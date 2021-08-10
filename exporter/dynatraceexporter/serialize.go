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

package dynatraceexporter

import (
	"errors"

	dtMetric "github.com/dynatrace-oss/dynatrace-metric-utils-go/metric"
	"github.com/dynatrace-oss/dynatrace-metric-utils-go/metric/dimensions"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/ttlmap"
	"go.opentelemetry.io/collector/model/pdata"
)

func serializeIntGauge(name, prefix string, dims dimensions.NormalizedDimensionList, dp pdata.IntDataPoint) (string, error) {
	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		dtMetric.WithIntGaugeValue(dp.Value()),
	)

	if err != nil {
		return "", err
	}

	return dm.Serialize()
}

func serializeGauge(name, prefix string, dims dimensions.NormalizedDimensionList, dp pdata.NumberDataPoint) (string, error) {
	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		dtMetric.WithFloatGaugeValue(dp.Value()),
	)

	if err != nil {
		return "", err
	}

	return dm.Serialize()
}

func serializeIntSum(name, prefix string, dims dimensions.NormalizedDimensionList, t pdata.AggregationTemporality, dp pdata.IntDataPoint, prev *ttlmap.TTLMap) (string, error) {
	var valueOpt dtMetric.MetricOption
	if t == pdata.AggregationTemporalityCumulative {
		dm, err := convertTotalIntCounterToDelta(name, prefix, dims, dp, prev)

		if err != nil {
			return "", err
		}

		if dm == nil {
			return "", nil
		}

		return dm.Serialize()
	}

	// unspecified temporality is treated as delta
	valueOpt = dtMetric.WithIntCounterValueDelta(dp.Value())

	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		valueOpt,
	)

	if err != nil {
		return "", err
	}

	return dm.Serialize()
}

func convertTotalIntCounterToDelta(name, prefix string, dims dimensions.NormalizedDimensionList, dp pdata.IntDataPoint, prev *ttlmap.TTLMap) (*dtMetric.Metric, error) {
	c := prev.Get(name)

	if c == nil {
		prev.Put(name, dp)
		return nil, nil
	}

	oldCount := c.(pdata.IntDataPoint)

	if oldCount.Timestamp().AsTime().After(dp.Timestamp().AsTime()) {
		// point is older than what we already have
		return nil, nil
	}

	val := dp.Value() - oldCount.Value()

	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		dtMetric.WithIntCounterValueDelta(val),
	)

	if err != nil {
		return dm, err
	}

	prev.Put(name, dp)

	return dm, err
}

func serializeSum(name, prefix string, dims dimensions.NormalizedDimensionList, t pdata.AggregationTemporality, dp pdata.NumberDataPoint, prev *ttlmap.TTLMap) (string, error) {
	var valueOpt dtMetric.MetricOption
	if t == pdata.AggregationTemporalityCumulative {
		dm, err := convertTotalCounterToDelta(name, prefix, dims, dp, prev)

		if err != nil {
			return "", err
		}

		if dm == nil {
			return "", nil
		}

		return dm.Serialize()
	}

	// unspecified temporality is treated as delta
	valueOpt = dtMetric.WithFloatCounterValueDelta(dp.Value())

	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		valueOpt,
	)

	if err != nil {
		return "", err
	}

	return dm.Serialize()
}

func convertTotalCounterToDelta(name, prefix string, dims dimensions.NormalizedDimensionList, dp pdata.NumberDataPoint, prev *ttlmap.TTLMap) (*dtMetric.Metric, error) {
	c := prev.Get(name)

	if c == nil {
		prev.Put(name, dp)
		return nil, nil
	}

	oldCount := c.(pdata.NumberDataPoint)

	if oldCount.Timestamp().AsTime().After(dp.Timestamp().AsTime()) {
		// point is older than what we already have
		return nil, nil
	}

	val := dp.Value() - oldCount.Value()

	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		dtMetric.WithFloatCounterValueDelta(val),
	)

	if err != nil {
		return dm, err
	}

	prev.Put(name, dp)

	return dm, err
}

func histMinMax(bounds []float64, counts []uint64) (float64, float64, bool) {
	// Because we do not know the actual min and max, we estimate them based on the min and max non-empty bucket
	minIdx, maxIdx := -1, -1
	for y := 0; y < len(counts); y++ {
		if counts[y] > 0 {
			if minIdx == -1 {
				minIdx = y
			}
			maxIdx = y
		}
	}

	if minIdx == -1 || maxIdx == -1 {
		return 0, 0, false
	}

	var min, max float64

	// Use lower bound for min unless it is the first bucket, then use upper
	if minIdx == 0 {
		min = bounds[minIdx]
	} else {
		min = bounds[minIdx-1]
	}

	// Use upper bound for max unless it is the last bucket, then use lower
	if maxIdx == len(counts)-1 {
		max = bounds[maxIdx-1]
	} else {
		max = bounds[maxIdx]
	}

	return min, max, true
}

func serializeHistogram(name, prefix string, dims dimensions.NormalizedDimensionList, t pdata.AggregationTemporality, dp pdata.HistogramDataPoint) (string, error) {
	if t == pdata.AggregationTemporalityCumulative {
		// convert to delta histogram
		// skip first point because there is nothing to calculate a delta from
		// what if bucket bounds change
		// TTL for cumulative histograms
		// reset detection? if cumulative and count decreases, the process probably reset
		return "", errors.New("cumulative histograms not supported")
	}

	min, max, nonEmpty := histMinMax(dp.ExplicitBounds(), dp.BucketCounts())

	if !nonEmpty {
		return "", nil
	}

	dm, err := dtMetric.NewMetric(
		name,
		dtMetric.WithPrefix(prefix),
		dtMetric.WithDimensions(dims),
		dtMetric.WithTimestamp(dp.Timestamp().AsTime()),
		dtMetric.WithFloatSummaryValue(
			min,
			max,
			dp.Sum(),
			int64(dp.Count()),
		),
	)

	if err != nil {
		return "", err
	}

	return dm.Serialize()
}
