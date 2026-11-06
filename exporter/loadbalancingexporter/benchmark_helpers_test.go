// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter/internal/metadata"
)

const (
	benchSpansPerTrace       = 5
	benchMetricNames         = 50
	benchDataPointsPerMetric = 2
)

type benchShape struct {
	name      string
	items     int
	resources int
	itemAttrs int
	resAttrs  int
}

var (
	shapeSmall = benchShape{name: "100i", items: 100, resources: 10, itemAttrs: 1, resAttrs: 1}
	shapeLarge = benchShape{name: "1000i", items: 1000, resources: 10, itemAttrs: 1, resAttrs: 1}
	shapeHeavy = benchShape{name: "1000i_heavy", items: 1000, resources: 10, itemAttrs: 20, resAttrs: 10}

	benchShapes         = []benchShape{shapeSmall, shapeLarge, shapeHeavy}
	benchEndpointCounts = []int{1, 10, 100}
	benchMultiEndpoints = []int{10, 100}
)

func benchEndpoints(n int) []string {
	endpoints := make([]string, n)
	for i := range n {
		endpoints[i] = fmt.Sprintf("endpoint-%d", i)
	}
	return endpoints
}

// benchTraceID returns a deterministic ID that is spread evenly over the hash ring using Fibonacci hashing.
// See Knuth, Multiplicative Hashing, The Art of Computer Programming, Vol. 3, p. 529.
func benchTraceID(i int) pcommon.TraceID {
	var id pcommon.TraceID
	binary.BigEndian.PutUint64(id[:8], uint64(i)*0x9E3779B97F4A7C15) // 2^64/phi
	binary.BigEndian.PutUint64(id[8:], uint64(i)+1)
	return id
}

// benchRoutingIDs returns IDs to cycle through, so lookups don't hit one cache-hot key.
func benchRoutingIDs() [][]byte {
	ids := make([][]byte, 1024)
	for i := range ids {
		id := benchTraceID(i)
		ids[i] = id[:]
	}
	return ids
}

func benchSpanID(i int) pcommon.SpanID {
	var id pcommon.SpanID
	binary.BigEndian.PutUint64(id[:], uint64(i)+1)
	return id
}

func fillBenchResource(attrs pcommon.Map, r int, s benchShape) {
	attrs.EnsureCapacity(s.resAttrs)
	attrs.PutStr("service.name", fmt.Sprintf("svc-%d", r))
	for k := 1; k < s.resAttrs; k++ {
		attrs.PutStr(fmt.Sprintf("resource.attr.%d", k), fmt.Sprintf("value-%d-%d", r, k))
	}
}

// variant makes attribute sets distinct, so streamID routing sees separate streams.
func fillBenchAttrs(attrs pcommon.Map, n, variant int) {
	attrs.EnsureCapacity(n)
	attrs.PutStr("http.method", "GET")
	for k := 1; k < n; k++ {
		key := fmt.Sprintf("attr.%d", k)
		switch k % 3 {
		case 0:
			attrs.PutStr(key, "some-reasonably-sized-attribute-value")
		case 1:
			attrs.PutInt(key, int64(variant*n+k))
		default:
			attrs.PutBool(key, k%2 == 0)
		}
	}
}

func benchTraces(s benchShape) ptrace.Traces {
	td := ptrace.NewTraces()
	scopes := make([]ptrace.SpanSlice, s.resources)
	for r := range s.resources {
		rs := td.ResourceSpans().AppendEmpty()
		fillBenchResource(rs.Resource().Attributes(), r, s)
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("bench")
		scopes[r] = ss.Spans()
	}
	for i := range s.items {
		// All spans of a trace share one resource, so service routing never splits a trace.
		t := i / benchSpansPerTrace
		span := scopes[t%s.resources].AppendEmpty()
		span.SetTraceID(benchTraceID(t))
		span.SetSpanID(benchSpanID(i))
		span.SetName("operation")
		span.SetKind(ptrace.SpanKindServer)
		fillBenchAttrs(span.Attributes(), s.itemAttrs, i)
	}
	return td
}

func benchLogs(s benchShape) plog.Logs {
	ld := plog.NewLogs()
	scopes := make([]plog.LogRecordSlice, s.resources)
	for r := range s.resources {
		rl := ld.ResourceLogs().AppendEmpty()
		fillBenchResource(rl.Resource().Attributes(), r, s)
		sl := rl.ScopeLogs().AppendEmpty()
		sl.Scope().SetName("bench")
		scopes[r] = sl.LogRecords()
	}
	for i := range s.items {
		t := i / benchSpansPerTrace
		lr := scopes[t%s.resources].AppendEmpty()
		lr.SetTraceID(benchTraceID(t))
		lr.SetSpanID(benchSpanID(i))
		lr.SetSeverityNumber(plog.SeverityNumberInfo)
		lr.Body().SetStr("request handled successfully")
		fillBenchAttrs(lr.Attributes(), s.itemAttrs, i)
	}
	return ld
}

func benchMetrics(s benchShape) pmetric.Metrics {
	md := pmetric.NewMetrics()
	scopes := make([]pmetric.MetricSlice, s.resources)
	for r := range s.resources {
		rm := md.ResourceMetrics().AppendEmpty()
		fillBenchResource(rm.Resource().Attributes(), r, s)
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("bench")
		scopes[r] = sm.Metrics()
	}
	for i := range s.items {
		m := scopes[i%s.resources].AppendEmpty()
		m.SetName(fmt.Sprintf("metric-%d", i%benchMetricNames))
		sum := m.SetEmptySum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		for d := range benchDataPointsPerMetric {
			dp := sum.DataPoints().AppendEmpty()
			dp.SetIntValue(int64(i + d))
			fillBenchAttrs(dp.Attributes(), s.itemAttrs, i*benchDataPointsPerMetric+d)
		}
	}
	return md
}

func benchConfig(routingKey string, endpoints int) *Config {
	cfg := &Config{
		Resolver: ResolverSettings{
			Static: configoptional.Some(StaticResolver{Hostnames: benchEndpoints(endpoints)}),
		},
		RoutingKey: routingKey,
	}
	// Config validation rejects attribute routing without attributes.
	if routingKey == attrRoutingStr {
		cfg.RoutingAttributes = []string{"service.name"}
	}
	return cfg
}

type benchSignal struct {
	name        string
	routingKeys []string
	setup       func(b *testing.B, cfg *Config, s benchShape) (consume func(context.Context) error, lb *loadBalancer)
}

var benchSignals = []benchSignal{
	{
		name:        "traces",
		routingKeys: []string{traceIDRoutingStr, svcRoutingStr, attrRoutingStr, randomnessRoutingStr},
		setup: func(b *testing.B, cfg *Config, s benchShape) (func(context.Context) error, *loadBalancer) {
			exp, err := newTracesExporter(exportertest.NewNopSettings(metadata.Type), cfg)
			require.NoError(b, err)
			// Replace the OTLP-backed load balancer built by the constructor.
			exp.loadBalancer = newBenchLoadBalancer(b, cfg, func() component.Component { return newNopMockTracesExporter() })
			startBenchComponent(b, exp)
			payload := benchTraces(s)
			return func(ctx context.Context) error { return exp.ConsumeTraces(ctx, payload) }, exp.loadBalancer
		},
	},
	{
		name:        "logs",
		routingKeys: []string{svcRoutingStr, traceIDRoutingStr, resourceRoutingStr, attrRoutingStr},
		setup: func(b *testing.B, cfg *Config, s benchShape) (func(context.Context) error, *loadBalancer) {
			exp, err := newLogsExporter(exportertest.NewNopSettings(metadata.Type), cfg)
			require.NoError(b, err)
			exp.loadBalancer = newBenchLoadBalancer(b, cfg, func() component.Component { return newNopMockLogsExporter() })
			startBenchComponent(b, exp)
			payload := benchLogs(s)
			return func(ctx context.Context) error { return exp.ConsumeLogs(ctx, payload) }, exp.loadBalancer
		},
	},
	{
		name:        "metrics",
		routingKeys: []string{svcRoutingStr, resourceRoutingStr, metricNameRoutingStr, streamIDRoutingStr, attrRoutingStr},
		setup: func(b *testing.B, cfg *Config, s benchShape) (func(context.Context) error, *loadBalancer) {
			exp, err := newMetricsExporter(exportertest.NewNopSettings(metadata.Type), cfg)
			require.NoError(b, err)
			exp.loadBalancer = newBenchLoadBalancer(b, cfg, func() component.Component { return newNopMockMetricsExporter() })
			startBenchComponent(b, exp)
			payload := benchMetrics(s)
			return func(ctx context.Context) error { return exp.ConsumeMetrics(ctx, payload) }, exp.loadBalancer
		},
	},
}

// newStartedBenchLoadBalancer returns a running load balancer over no-op backends.
func newStartedBenchLoadBalancer(b *testing.B, endpoints int) *loadBalancer {
	b.Helper()
	lb := newBenchLoadBalancer(b, benchConfig(traceIDRoutingStr, endpoints), func() component.Component { return newNopMockTracesExporter() })
	startBenchComponent(b, lb)
	return lb
}

func newBenchLoadBalancer(b *testing.B, cfg *Config, newBackend func() component.Component) *loadBalancer {
	b.Helper()
	ts, tb := getTelemetryAssets(b)
	lb, err := newLoadBalancer(ts.Logger, cfg, func(context.Context, string) (component.Component, error) {
		return newBackend(), nil
	}, tb)
	require.NoError(b, err)
	return lb
}

func startBenchComponent(b *testing.B, c component.Component) {
	b.Helper()
	require.NoError(b, c.Start(b.Context(), componenttest.NewNopHost()))
	b.Cleanup(func() { require.NoError(b, c.Shutdown(b.Context())) })
}

// benchLoop runs op once per iteration with allocation reporting.
func benchLoop(b *testing.B, op func() error) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		if err := op(); err != nil {
			b.Fatal(err)
		}
	}
}

func forEachEndpointCount(b *testing.B, fn func(b *testing.B, endpoints int)) {
	for _, endpoints := range benchEndpointCounts {
		b.Run(fmt.Sprintf("%dE", endpoints), func(b *testing.B) { fn(b, endpoints) })
	}
}

type splitCase[T any] struct {
	name  string
	split func(T) map[string]T
}

func benchSplit[T any](b *testing.B, newPayload func(benchShape) T, cases []splitCase[T]) {
	for _, tc := range cases {
		for _, shape := range benchShapes {
			b.Run(fmt.Sprintf("%s/%s", tc.name, shape.name), func(b *testing.B) {
				payload := newPayload(shape)
				benchLoop(b, func() error {
					tc.split(payload)
					return nil
				})
			})
		}
	}
}
