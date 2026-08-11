// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter"

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"
)

// wrappedExporter is an exporter that waits for the data processing to complete before shutting
// down, for as long as the shutdown context allows. If that context expires first the component is
// shut down anyway and the deadline error returned alongside, so a caller that must not cut off an
// in-flight export has to pass a context at least as generous as its export timeout.
// consumeWG has to be incremented explicitly by the consumer of the wrapped exporter.
type wrappedExporter struct {
	component.Component
	consumeWG sync.WaitGroup

	endpoint string

	// we store the attributes here for both cases, to avoid new allocations on the hot path
	endpointAttr attribute.Set
	successAttr  attribute.Set
	failureAttr  attribute.Set
}

func newWrappedExporter(exp component.Component, identifier string) *wrappedExporter {
	ea := attribute.String("endpoint", identifier)
	return &wrappedExporter{
		Component:    exp,
		endpoint:     identifier,
		endpointAttr: attribute.NewSet(ea),
		successAttr:  attribute.NewSet(ea, attribute.Bool("success", true)),
		failureAttr:  attribute.NewSet(ea, attribute.Bool("success", false)),
	}
}

func (we *wrappedExporter) Shutdown(ctx context.Context) error {
	// consumeWG may never reach zero if a consumer to this backend is stuck, so the wait respects
	// ctx. WaitGroup has no cancellable wait, so this goroutine then outlives Shutdown: one leaked
	// goroutine per permanently wedged backend, preferable to blocking the collector's shutdown.
	done := make(chan struct{})

	go func() {
		we.consumeWG.Wait()
		close(done)
	}()

	var waitErr error
	select {
	case <-done:
	case <-ctx.Done():
		// select picks at random when both are ready, so re-check the drain before blaming the
		// deadline for data that in fact made it out.
		select {
		case <-done:
		default:
			waitErr = fmt.Errorf("shutting down %q with data still in flight: %w", we.endpoint, ctx.Err())
		}
	}

	return errors.Join(waitErr, we.Component.Shutdown(ctx))
}

func (we *wrappedExporter) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	te, ok := we.Component.(exporter.Traces)
	if !ok {
		return fmt.Errorf("unable to export traces, unexpected exporter type: expected exporter.Traces but got %T", we.Component)
	}
	return te.ConsumeTraces(ctx, td)
}

func (we *wrappedExporter) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	me, ok := we.Component.(exporter.Metrics)
	if !ok {
		return fmt.Errorf("unable to export metrics, unexpected exporter type: expected exporter.Metrics but got %T", we.Component)
	}
	return me.ConsumeMetrics(ctx, md)
}

func (we *wrappedExporter) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	le, ok := we.Component.(exporter.Logs)
	if !ok {
		return fmt.Errorf("unable to export logs, unexpected exporter type: expected exporter.Logs but got %T", we.Component)
	}
	return le.ConsumeLogs(ctx, ld)
}
