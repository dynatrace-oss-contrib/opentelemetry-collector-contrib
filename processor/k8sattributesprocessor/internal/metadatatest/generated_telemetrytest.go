// Code generated by mdatagen. DO NOT EDIT.

package metadatatest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"
)

type Telemetry struct {
	componenttest.Telemetry
}

func SetupTelemetry(opts ...componenttest.TelemetryOption) Telemetry {
	return Telemetry{Telemetry: componenttest.NewTelemetry(opts...)}
}

func (tt *Telemetry) NewSettings() processor.Settings {
	set := processortest.NewNopSettings()
	set.ID = component.NewID(component.MustNewType("k8sattributes"))
	set.TelemetrySettings = tt.NewTelemetrySettings()
	return set
}

func (tt *Telemetry) AssertMetrics(t *testing.T, expected []metricdata.Metrics, opts ...metricdatatest.Option) {
	var md metricdata.ResourceMetrics
	require.NoError(t, tt.Reader.Collect(context.Background(), &md))
	// ensure all required metrics are present
	for _, want := range expected {
		got := getMetricFromResource(want.Name, md)
		metricdatatest.AssertEqual(t, want, got, opts...)
	}

	// ensure no additional metrics are emitted
	require.Equal(t, len(expected), lenMetrics(md))
}

func AssertEqualOtelsvcK8sIPLookupMiss(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_ip_lookup_miss",
		Description: "Number of times pod by IP lookup failed.",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_ip_lookup_miss")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sNamespaceAdded(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_namespace_added",
		Description: "Number of namespace add events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_namespace_added")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sNamespaceDeleted(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_namespace_deleted",
		Description: "Number of namespace delete events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_namespace_deleted")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sNamespaceUpdated(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_namespace_updated",
		Description: "Number of namespace update events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_namespace_updated")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sNodeAdded(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_node_added",
		Description: "Number of node add events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_node_added")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sNodeDeleted(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_node_deleted",
		Description: "Number of node delete events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_node_deleted")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sNodeUpdated(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_node_updated",
		Description: "Number of node update events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_node_updated")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sPodAdded(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_pod_added",
		Description: "Number of pod add events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_pod_added")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sPodDeleted(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_pod_deleted",
		Description: "Number of pod delete events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_pod_deleted")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sPodTableSize(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_pod_table_size",
		Description: "Size of table containing pod info",
		Unit:        "1",
		Data: metricdata.Gauge[int64]{
			DataPoints: dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_pod_table_size")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sPodUpdated(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_pod_updated",
		Description: "Number of pod update events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_pod_updated")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sReplicasetAdded(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_replicaset_added",
		Description: "Number of ReplicaSet add events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_replicaset_added")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sReplicasetDeleted(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_replicaset_deleted",
		Description: "Number of ReplicaSet delete events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_replicaset_deleted")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualOtelsvcK8sReplicasetUpdated(t *testing.T, tt componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_otelsvc_k8s_replicaset_updated",
		Description: "Number of ReplicaSet update events received",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got := getMetric(t, tt, "otelcol_otelsvc_k8s_replicaset_updated")
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func getMetric(t *testing.T, tt componenttest.Telemetry, name string) metricdata.Metrics {
	var md metricdata.ResourceMetrics
	require.NoError(t, tt.Reader.Collect(context.Background(), &md))
	return getMetricFromResource(name, md)
}

func getMetricFromResource(name string, got metricdata.ResourceMetrics) metricdata.Metrics {
	for _, sm := range got.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}

	return metricdata.Metrics{}
}

func lenMetrics(got metricdata.ResourceMetrics) int {
	metricsCount := 0
	for _, sm := range got.ScopeMetrics {
		metricsCount += len(sm.Metrics)
	}

	return metricsCount
}
