// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

import (
	"fmt"
	"testing"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func BenchmarkRouting(b *testing.B) {
	for _, sig := range benchSignals {
		for _, rk := range sig.routingKeys {
			for _, endpoints := range benchEndpointCounts {
				for _, shape := range benchShapes {
					b.Run(fmt.Sprintf("%s/%s/%dE_%s", sig.name, rk, endpoints, shape.name), func(b *testing.B) {
						consume, _ := sig.setup(b, benchConfig(rk, endpoints), shape)
						benchLoop(b, func() error { return consume(b.Context()) })
					})
				}
			}
		}
	}
}

func BenchmarkRoutingParallel(b *testing.B) {
	for _, sig := range benchSignals {
		for _, rk := range sig.routingKeys {
			for _, endpoints := range benchMultiEndpoints {
				b.Run(fmt.Sprintf("%s/%s/%dE_%s", sig.name, rk, endpoints, shapeLarge.name), func(b *testing.B) {
					consume, _ := sig.setup(b, benchConfig(rk, endpoints), shapeLarge)
					b.ReportAllocs()
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							if err := consume(b.Context()); err != nil {
								b.Error(err) // Fatal must not be called from RunParallel goroutines.
								return
							}
						}
					})
				})
			}
		}
	}
}

// Flips between N-1 and N endpoints every `every` iterations. onBackendChanges is
// called directly because the static resolver never changes on its own.
func BenchmarkRoutingDuringChurn(b *testing.B) {
	for _, sig := range benchSignals {
		rk := sig.routingKeys[0]
		for _, endpoints := range benchMultiEndpoints {
			for _, every := range []int{1, 10, 100} {
				b.Run(fmt.Sprintf("%s/%s/%dE_every%d", sig.name, rk, endpoints, every), func(b *testing.B) {
					consume, lb := sig.setup(b, benchConfig(rk, endpoints), shapeLarge)
					full := benchEndpoints(endpoints)
					backends := [2][]string{full[:endpoints-1], full}
					i := 0
					benchLoop(b, func() error {
						if i%every == 0 {
							lb.onBackendChanges(backends[(i/every)%2])
						}
						i++
						return consume(b.Context())
					})
				})
			}
		}
	}
}

// Hot-path micro benchmarks keep an inline loop so a closure call doesn't skew ns/op.
func BenchmarkHashRingEndpointFor(b *testing.B) {
	forEachEndpointCount(b, func(b *testing.B, endpoints int) {
		ring := newHashRing(benchEndpoints(endpoints))
		ids := benchRoutingIDs()
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			_ = ring.endpointFor(ids[i%len(ids)])
		}
	})
}

func BenchmarkExporterAndEndpoint(b *testing.B) {
	forEachEndpointCount(b, func(b *testing.B, endpoints int) {
		lb := newStartedBenchLoadBalancer(b, endpoints)
		ids := benchRoutingIDs()
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			if _, _, err := lb.exporterAndEndpoint(ids[i%len(ids)]); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkNewHashRing(b *testing.B) {
	forEachEndpointCount(b, func(b *testing.B, endpoints int) {
		eps := benchEndpoints(endpoints)
		b.ReportAllocs()
		for b.Loop() {
			_ = newHashRing(eps)
		}
	})
}

func BenchmarkRoutingIdentifiersFromTraces(b *testing.B) {
	for _, tc := range []struct {
		name  string
		key   routingKey
		attrs []string
	}{
		{name: svcRoutingStr, key: svcRouting},
		{name: attrRoutingStr, key: attrRouting, attrs: []string{"service.name"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			// ConsumeTraces calls this once per trace, after SplitTraces, so the input is a single trace.
			// The function only reads the first span and the routing attributes, so payload size doesn't matter.
			shape := shapeLarge
			shape.items, shape.resources = benchSpansPerTrace, 1
			td := benchTraces(shape)
			benchLoop(b, func() error {
				_, err := routingIdentifiersFromTraces(td, tc.key, tc.attrs)
				return err
			})
		})
	}
}

func BenchmarkSplitLogs(b *testing.B) {
	benchSplit(b, benchLogs, []splitCase[plog.Logs]{
		{name: svcRoutingStr, split: splitLogsByServiceName},
		{name: resourceRoutingStr, split: splitLogsByResourceID},
		{name: traceIDRoutingStr, split: splitLogsByTraceID},
		{name: attrRoutingStr, split: func(ld plog.Logs) map[string]plog.Logs {
			return splitLogsByAttributes(ld, []string{"service.name"})
		}},
	})
}

func BenchmarkSplitMetrics(b *testing.B) {
	benchSplit(b, benchMetrics, []splitCase[pmetric.Metrics]{
		{name: svcRoutingStr, split: func(md pmetric.Metrics) map[string]pmetric.Metrics {
			m, _ := splitMetricsByResourceServiceName(md) // errors only for missing service.name
			return m
		}},
		{name: resourceRoutingStr, split: splitMetricsByResourceID},
		{name: metricNameRoutingStr, split: splitMetricsByMetricName},
		{name: streamIDRoutingStr, split: splitMetricsByStreamID},
		{name: attrRoutingStr, split: func(md pmetric.Metrics) map[string]pmetric.Metrics {
			return splitMetricsByAttributes(md, []string{"service.name"})
		}},
	})
}
