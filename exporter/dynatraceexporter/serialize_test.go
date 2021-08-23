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
	"testing"
	"time"

	"github.com/dynatrace-oss/dynatrace-metric-utils-go/metric/dimensions"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/model/pdata"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/ttlmap"
)

func Test_serializeGauge(t *testing.T) {
	t.Run("float with prefix and dimension", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetDoubleVal(5.5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		got, err := serializeGauge("dbl_gauge", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), dp)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.dbl_gauge,key=value gauge,5.5 1626438600000", got)
	})

	t.Run("int with prefix and dimension", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		got, err := serializeGauge("int_gauge", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), dp)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.int_gauge,key=value gauge,5 1626438600000", got)
	})

	t.Run("without timestamp", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)

		got, err := serializeGauge("int_gauge", "prefix", dimensions.NewNormalizedDimensionList(), dp)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.int_gauge gauge,5", got)
	})
}

func Test_serializeSum(t *testing.T) {
	t.Run("without timestamp", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)

		got, err := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(), pdata.AggregationTemporalityDelta, dp, ttlmap.New(1, 1))
		assert.NoError(t, err)
		assert.Equal(t, "prefix.int_sum count,delta=5", got)
	})

	t.Run("float delta with prefix and dimension", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetDoubleVal(5.5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		prev := ttlmap.New(1, 1)

		got, err := serializeSum("double_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityDelta, dp, prev)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.double_sum,key=value count,delta=5.5 1626438600000", got)
	})

	t.Run("int delta with prefix and dimension", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		got, err := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityDelta, dp, ttlmap.New(1, 1))
		assert.NoError(t, err)
		assert.Equal(t, "prefix.int_sum,key=value count,delta=5 1626438600000", got)
	})

	t.Run("float cumulative with prefix and dimension", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetDoubleVal(5.5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		dp2 := pdata.NewNumberDataPoint()
		dp2.SetDoubleVal(7.0)
		dp2.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 31, 0, 0, time.UTC).UnixNano()))

		prev := ttlmap.New(1, 1)

		got, err := serializeSum("double_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, dp, prev)
		assert.NoError(t, err)
		assert.Equal(t, "", got)

		got, err = serializeSum("double_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, dp2, prev)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.double_sum,key=value count,delta=1.5 1626438660000", got)
	})

	t.Run("int cumulative with prefix and dimension", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		dp2 := pdata.NewNumberDataPoint()
		dp2.SetIntVal(10)
		dp2.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 31, 0, 0, time.UTC).UnixNano()))

		prev := ttlmap.New(1, 1)

		got, err := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, dp, prev)
		assert.NoError(t, err)
		assert.Equal(t, "", got)

		got, err = serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, dp2, prev)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.int_sum,key=value count,delta=5 1626438660000", got)
	})

	t.Run("different dimensions should be treated as separate counters", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)
		dp.Attributes().Insert("sort", pdata.NewAttributeValueString("unstable"))
		dp.Attributes().Insert("group", pdata.NewAttributeValueString("a"))
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		dp2 := pdata.NewNumberDataPoint()
		dp2.SetIntVal(10)
		dp2.Attributes().Insert("sort", pdata.NewAttributeValueString("unstable"))
		dp2.Attributes().Insert("group", pdata.NewAttributeValueString("b"))
		dp2.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		dp3 := pdata.NewNumberDataPoint()
		dp3.SetIntVal(10)
		dp3.Attributes().Insert("group", pdata.NewAttributeValueString("a"))
		dp3.Attributes().Insert("sort", pdata.NewAttributeValueString("unstable"))
		dp3.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		dp4 := pdata.NewNumberDataPoint()
		dp4.SetIntVal(20)
		dp4.Attributes().Insert("group", pdata.NewAttributeValueString("b"))
		dp4.Attributes().Insert("sort", pdata.NewAttributeValueString("unstable"))
		dp4.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		prev := ttlmap.New(1, 1)

		got, err := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "a")), pdata.AggregationTemporalityCumulative, dp, prev)
		got2, err2 := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "b")), pdata.AggregationTemporalityCumulative, dp2, prev)
		got3, err3 := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "a")), pdata.AggregationTemporalityCumulative, dp3, prev)
		got4, err4 := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "b")), pdata.AggregationTemporalityCumulative, dp4, prev)

		assert.NoError(t, err)
		assert.NoError(t, err2)
		assert.NoError(t, err3)
		assert.NoError(t, err4)
		assert.Equal(t, "", got)
		assert.Equal(t, "", got2)
		assert.Equal(t, "prefix.int_sum,key=a count,delta=5 1626438600000", got3)
		assert.Equal(t, "prefix.int_sum,key=b count,delta=10 1626438600000", got4)
	})

	t.Run("count values older than the previous count value are dropped", func(t *testing.T) {
		dp := pdata.NewNumberDataPoint()
		dp.SetIntVal(5)
		dp.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

		dp2 := pdata.NewNumberDataPoint()
		dp2.SetIntVal(5)
		dp2.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 29, 0, 0, time.UTC).UnixNano()))

		prev := ttlmap.New(1, 1)

		got, err := serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, dp, prev)
		assert.NoError(t, err)
		assert.Equal(t, "", got)

		assert.Equal(t, dp, prev.Get("int_sum"))

		got, err = serializeSum("int_sum", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, dp2, prev)
		assert.NoError(t, err)
		assert.Equal(t, "", got)

		assert.Equal(t, dp, prev.Get("int_sum"))
	})
}

func Test_serializeHistogram(t *testing.T) {
	hist := pdata.NewHistogramDataPoint()
	hist.SetExplicitBounds([]float64{0, 2, 4, 8})
	hist.SetBucketCounts([]uint64{0, 1, 0, 1, 0})
	hist.SetCount(2)
	hist.SetSum(9.5)
	hist.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

	histWithNonEmptyFirstLast := pdata.NewHistogramDataPoint()
	histWithNonEmptyFirstLast.SetExplicitBounds([]float64{0, 2, 4, 8})
	histWithNonEmptyFirstLast.SetBucketCounts([]uint64{0, 1, 0, 1, 1})
	histWithNonEmptyFirstLast.SetCount(3)
	histWithNonEmptyFirstLast.SetSum(9.5)
	histWithNonEmptyFirstLast.SetTimestamp(pdata.Timestamp(time.Date(2021, 07, 16, 12, 30, 0, 0, time.UTC).UnixNano()))

	t.Run("delta with prefix and dimension", func(t *testing.T) {
		got, err := serializeHistogram("delta_hist", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityDelta, hist)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.delta_hist,key=value gauge,min=0,max=8,sum=9.5,count=2 1626438600000", got)
	})

	t.Run("delta with non-empty first and last bucket", func(t *testing.T) {
		got, err := serializeHistogram("delta_nonempty_first_last_hist", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityDelta, histWithNonEmptyFirstLast)
		assert.NoError(t, err)
		assert.Equal(t, "prefix.delta_nonempty_first_last_hist,key=value gauge,min=0,max=8,sum=9.5,count=3 1626438600000", got)
	})

	t.Run("cumulative with prefix and dimension", func(t *testing.T) {
		got, err := serializeHistogram("hist", "prefix", dimensions.NewNormalizedDimensionList(dimensions.NewDimension("key", "value")), pdata.AggregationTemporalityCumulative, hist)
		assert.Error(t, err)
		assert.Equal(t, "", got)
	})
}
