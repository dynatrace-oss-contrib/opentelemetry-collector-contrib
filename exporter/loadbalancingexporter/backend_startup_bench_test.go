// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

import (
	"fmt"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exportertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter/internal/metadata"
)

// newRealLoadBalancer returns a load balancer wired to the real OTLP exporter factory, using the
// default configuration (sending queue enabled).
func newRealLoadBalancer(tb testing.TB) *loadBalancer {
	cfg := createDefaultConfig().(*Config)
	cfg.Resolver.Static = configoptional.Some(StaticResolver{Hostnames: []string{"placeholder"}})
	exp, err := newTracesExporter(exportertest.NewNopSettings(metadata.Type), cfg)
	require.NoError(tb, err)
	exp.loadBalancer.host = componenttest.NewNopHost()
	return exp.loadBalancer
}

func listen(tb testing.TB) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func reportPercentiles(b *testing.B, durations []time.Duration) {
	if len(durations) == 0 {
		return
	}
	slices.Sort(durations)
	pct := func(p float64) float64 {
		return float64(durations[int(float64(len(durations)-1)*p)].Microseconds())
	}
	b.ReportMetric(pct(0.50), "p50-µs")
	b.ReportMetric(pct(0.99), "p99-µs")
	b.ReportMetric(float64(durations[len(durations)-1].Microseconds()), "max-µs")
}

// BenchmarkBackendStartup measures the creation and Start of a single backend exporter, which is
// the work bounded by backendStartTimeout.
func BenchmarkBackendStartup(b *testing.B) {
	for _, tc := range []struct{ name, endpoint string }{
		{"reachable", listen(b)},
		{"blackhole", "10.255.255.1:4317"},
		{"unresolvable", "does-not-exist.invalid:4317"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			lb := newRealLoadBalancer(b)
			var durations []time.Duration
			for b.Loop() {
				start := time.Now()
				we, err := lb.startExporter(tc.endpoint)
				require.NoError(b, err)
				durations = append(durations, time.Since(start))

				b.StopTimer()
				require.NoError(b, we.Shutdown(b.Context()))
				b.StartTimer()
			}
			reportPercentiles(b, durations)
		})
	}
}

// BenchmarkOnBackendChangesRealExporters measures a whole topology change that adds n backends,
// i.e. the time backendChangeMu is held.
func BenchmarkOnBackendChangesRealExporters(b *testing.B) {
	for _, n := range []int{1, 10, 100} {
		endpoints := make([]string, n)
		for i := range endpoints {
			endpoints[i] = listen(b)
		}
		b.Run(fmt.Sprintf("backends=%d", n), func(b *testing.B) {
			var durations []time.Duration
			for b.Loop() {
				b.StopTimer()
				lb := newRealLoadBalancer(b)
				b.StartTimer()

				start := time.Now()
				lb.onBackendChanges(endpoints)
				durations = append(durations, time.Since(start))

				b.StopTimer()
				require.Len(b, lb.exporters, n)
				for _, e := range lb.exporters {
					require.NoError(b, e.Shutdown(b.Context()))
				}
				b.StartTimer()
			}
			reportPercentiles(b, durations)
		})
	}
}
