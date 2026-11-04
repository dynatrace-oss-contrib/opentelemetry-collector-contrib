// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// brokenBackendLB returns a load balancer whose ring contains "good" and "bad", where the
// exporter for "bad" failed to be created, as happens when addMissingExporters fails.
func brokenBackendLB(t *testing.T, good func() component.Component) (*loadBalancer, *Config) {
	ts, tb := getTelemetryAssets(t)
	factory := func(_ context.Context, endpoint string) (component.Component, error) {
		if endpoint == "good:4317" {
			return good(), nil
		}
		return nil, errors.New("cannot create exporter")
	}
	cfg := &Config{
		Resolver:   ResolverSettings{Static: configoptional.Some(StaticResolver{Hostnames: []string{"good"}})},
		RoutingKey: svcRoutingStr,
	}
	lb, err := newLoadBalancer(ts.Logger, cfg, factory, tb)
	require.NoError(t, err)
	lb.onBackendChanges([]string{"good", "bad"})
	require.Len(t, lb.exporters, 1)
	return lb, cfg
}

func shutdownReturns(t *testing.T, exp *wrappedExporter) bool {
	done := make(chan struct{})
	go func() {
		_ = exp.Shutdown(context.Background())
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

func TestReproLogsConsumeWGLeak(t *testing.T) {
	lb, cfg := brokenBackendLB(t, func() component.Component { return newNopMockLogsExporter() })
	ts, _ := getTelemetryAssets(t)
	p, err := newLogsExporter(ts, cfg)
	require.NoError(t, err)
	p.loadBalancer = lb

	var errs int
	for range 20 {
		ld := plog.NewLogs()
		for i := range 50 {
			simpleLogWithServiceName(fmt.Sprintf("svc-%d", i)).ResourceLogs().MoveAndAppendTo(ld.ResourceLogs())
		}
		if p.ConsumeLogs(t.Context(), ld) != nil {
			errs++
		}
	}
	require.Positive(t, errs, "expected resolution errors")
	require.True(t, shutdownReturns(t, lb.exporters["good:4317"]), "Shutdown of the healthy backend hangs")
}

func TestReproMetricsConsumeWGLeak(t *testing.T) {
	lb, cfg := brokenBackendLB(t, func() component.Component { return newNopMockMetricsExporter() })
	ts, _ := getTelemetryAssets(t)
	p, err := newMetricsExporter(ts, cfg)
	require.NoError(t, err)
	p.loadBalancer = lb

	var errs int
	for range 20 {
		md := pmetric.NewMetrics()
		for i := range 50 {
			rm := md.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", fmt.Sprintf("svc-%d", i))
			appendSimpleMetricWithID(rm, fmt.Sprintf("m-%d", i))
		}
		if p.ConsumeMetrics(t.Context(), md) != nil {
			errs++
		}
	}
	require.Positive(t, errs, "expected resolution errors")
	require.True(t, shutdownReturns(t, lb.exporters["good:4317"]), "Shutdown of the healthy backend hangs")
}

// Control: traces already release the counts on main.
func TestReproTracesNoLeak(t *testing.T) {
	lb, cfg := brokenBackendLB(t, func() component.Component { return newNopMockTracesExporter() })
	ts, _ := getTelemetryAssets(t)
	p, err := newTracesExporter(ts, cfg)
	require.NoError(t, err)
	p.loadBalancer = lb

	var errs int
	for range 20 {
		td := ptrace.NewTraces()
		for i := range 50 {
			rs := td.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", fmt.Sprintf("svc-%d", i))
			rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID([16]byte{byte(i + 1)})
		}
		if p.ConsumeTraces(t.Context(), td) != nil {
			errs++
		}
	}
	require.Positive(t, errs, "expected resolution errors")
	require.True(t, shutdownReturns(t, lb.exporters["good:4317"]), "Shutdown of the healthy backend hangs")
}
