package instrument_serialization

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dynatrace-metric-utils-go/metric/dimensions"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap/zaptest/observer"
)

func Test_makeCombinedDimensions(t *testing.T) {
	defaultDims := dimensions.NewNormalizedDimensionList(
		dimensions.NewDimension("a", "default"),
		dimensions.NewDimension("b", "default"),
		dimensions.NewDimension("c", "default"),
	)
	attributes := pcommon.NewMap()
	attributes.Insert("a", pcommon.NewValueString("attribute"))
	attributes.Insert("b", pcommon.NewValueString("attribute"))
	staticDims := dimensions.NewNormalizedDimensionList(
		dimensions.NewDimension("a", "static"),
	)
	expected := dimensions.NewNormalizedDimensionList(
		dimensions.NewDimension("a", "static"),
		dimensions.NewDimension("b", "attribute"),
		dimensions.NewDimension("c", "default"),
	)

	actual := makeCombinedDimensions(defaultDims, attributes, staticDims)

	sortAndStringify :=
		func(dims []dimensions.Dimension) string {
			sort.Slice(dims, func(i, j int) bool {
				return dims[i].Key < dims[j].Key
			})
			tokens := make([]string, len(dims))
			for i, dim := range dims {
				tokens[i] = fmt.Sprintf("%s=%s", dim.Key, dim.Value)
			}
			return strings.Join(tokens, ",")
		}

	assert.Equal(t, actual.Format(sortAndStringify), expected.Format(sortAndStringify))
}

type simplifiedLogRecord struct {
	message    string
	attributes map[string]string
}

func makeSimplifiedLogRecordsFromObservedLogs(observedLogs *observer.ObservedLogs) []simplifiedLogRecord {
	observedLogRecords := make([]simplifiedLogRecord, observedLogs.Len())

	for i, observedLog := range observedLogs.All() {
		contextStringMap := make(map[string]string, len(observedLog.ContextMap()))
		for k, v := range observedLog.ContextMap() {
			contextStringMap[k] = fmt.Sprint(v)
		}
		observedLogRecords[i] = simplifiedLogRecord{
			message:    observedLog.Message,
			attributes: contextStringMap,
		}
	}
	return observedLogRecords
}
