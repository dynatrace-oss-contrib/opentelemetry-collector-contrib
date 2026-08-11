// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrappedExporterShutdown_RespectsContext_WhenConsumeNeverCompletes(t *testing.T) {
	var shutdownCalled atomic.Bool
	we := newWrappedExporter(mockComponent{ShutdownFunc: func(context.Context) error {
		shutdownCalled.Store(true)
		return nil
	}}, "mock")
	we.consumeWG.Add(1)
	defer we.consumeWG.Done()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	var err error
	runWithinTimeout(t, 5*time.Second, func() {
		err = we.Shutdown(ctx)
	})

	assert.True(t, shutdownCalled.Load())
	// Cutting off an in-flight export is a reportable event, not something to swallow.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "mock")
}

func TestWrappedExporterShutdown_DrainsBeforeShuttingDown(t *testing.T) {
	consumeReturned := make(chan struct{})
	var shutdownBeforeDrain atomic.Bool

	we := newWrappedExporter(mockComponent{ShutdownFunc: func(context.Context) error {
		select {
		case <-consumeReturned:
		default:
			shutdownBeforeDrain.Store(true)
		}
		return nil
	}}, "mock")

	we.consumeWG.Add(1)
	time.AfterFunc(50*time.Millisecond, func() {
		close(consumeReturned)
		we.consumeWG.Done()
	})

	require.NoError(t, we.Shutdown(t.Context()))
	assert.False(t, shutdownBeforeDrain.Load())
}
