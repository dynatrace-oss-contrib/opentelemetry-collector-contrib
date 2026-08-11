// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

// Tests for the load balancer's backend lifecycle: how exporters are added when the resolver
// reports a change, how a slow or failing backend is kept from stalling the data path, and how
// Shutdown tears the whole set down.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
)

func TestNewLoadBalancer_ExporterTimeouts_UseConfiguredValues(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	cfg := simpleConfig()
	cfg.ExporterAddTimeout = 7 * time.Second
	cfg.ExporterShutdownTimeout = 3 * time.Second

	p, err := newLoadBalancer(ts.Logger, cfg, nil, tb)
	require.NoError(t, err)

	assert.Equal(t, 7*time.Second, p.exporterAddTimeout)
	assert.Equal(t, 3*time.Second, p.exporterShutdownTimeout)
}

func TestCreateDefaultConfig_ExporterTimeouts(t *testing.T) {
	cfg := createDefaultConfig().(*Config)

	assert.Equal(t, defaultExporterAddTimeout, cfg.ExporterAddTimeout)
	assert.Equal(t, defaultExporterShutdownTimeout, cfg.ExporterShutdownTimeout)
}

func TestNewLoadBalancer_ExporterTimeouts_ZeroMeansUnbounded(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	cfg := simpleConfig()
	cfg.ExporterAddTimeout = 0
	cfg.ExporterShutdownTimeout = 0

	p, err := newLoadBalancer(ts.Logger, cfg, nil, tb)
	require.NoError(t, err)

	// createDefaultConfig supplies the defaults, so a zero reaching newLoadBalancer is an explicit
	// "wait as long as it takes" and must not be silently rewritten to the default.
	assert.Zero(t, p.exporterAddTimeout)
	assert.Zero(t, p.exporterShutdownTimeout)
}

func TestAwaitExporterBatch_UnboundedAddTimeout_WaitsForSlowBackend(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), blockingFactory("slow", release, nil), tb)
	require.NoError(t, err)
	require.Zero(t, p.exporterAddTimeout)

	time.AfterFunc(50*time.Millisecond, func() { close(release) })

	p.onBackendChanges([]string{"slow"})

	assert.Equal(t, 1, p.NumBackends())
	assertRoutable(t, p, "slow")
}

func TestOnBackendChanges_SlowAdd_DoesNotBlockCallerOrDataPath(t *testing.T) {
	tests := []struct {
		name    string
		factory func(release <-chan struct{}) componentFactory
	}{
		{
			name: "factory observes context cancellation",
			factory: func(release <-chan struct{}) componentFactory {
				endpoint := endpointWithPort("hanging")
				return func(ctx context.Context, ep string) (component.Component, error) {
					if ep != endpoint {
						return newNopMockExporter(), nil
					}
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-release:
						return newNopMockExporter(), nil
					}
				}
			},
		},
		{
			name: "factory ignores context cancellation",
			factory: func(release <-chan struct{}) componentFactory {
				return blockingFactory("hanging", release, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, tb := getTelemetryAssets(t)
			release := make(chan struct{})

			p, err := newLoadBalancer(ts.Logger, simpleConfig(), tt.factory(release), tb)
			require.NoError(t, err)
			p.exporterAddTimeout = 50 * time.Millisecond

			p.onBackendChanges([]string{"endpoint-1"})
			require.Equal(t, 1, p.NumBackends())

			runWithinTimeout(t, 5*time.Second, func() {
				p.onBackendChanges([]string{"endpoint-1", "hanging"})
			})
			assert.Equal(t, 1, p.NumBackends())

			// The backend that missed the deadline stays out of the ring, so every identifier
			// keeps routing to the one that is ready.
			runWithinTimeout(t, 5*time.Second, func() {
				assertRoutable(t, p, "endpoint-1")
			})

			// It joins the ring when it finishes, with no further resolver change.
			close(release)
			p.exportersAddWG.Wait()

			assert.Equal(t, 2, p.NumBackends())
			assertRoutable(t, p, "endpoint-1", "hanging")
		})
	}
}

func TestCreateAndInstallExporter_StartFails_ShutsDownTheComponent(t *testing.T) {
	ts, tb := getTelemetryAssets(t)

	var shutdowns atomic.Int32
	factory := func(context.Context, string) (component.Component, error) {
		return mockComponent{
			StartFunc: func(context.Context, component.Host) error {
				return errors.New("start failed")
			},
			ShutdownFunc: func(context.Context) error {
				shutdowns.Add(1)
				return nil
			},
		}, nil
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)

	// The endpoint is released for a retry on every resolution, so a component left un-shut-down
	// here leaks once per resolution for as long as the backend keeps failing.
	p.onBackendChanges([]string{"broken"})
	p.onBackendChanges([]string{"broken"})

	require.Equal(t, 0, p.NumBackends())
	p.exportersShutdownWG.Wait()
	assert.Equal(t, int32(2), shutdowns.Load())
}

func TestOnBackendChanges_FailedAdd_RetriedOnNextResolution(t *testing.T) {
	ts, tb := getTelemetryAssets(t)

	var calls atomic.Int32
	factory := func(_ context.Context, ep string) (component.Component, error) {
		if ep != endpointWithPort("flaky") {
			return newNopMockExporter(), nil
		}
		if calls.Add(1) == 1 {
			return nil, errors.New("transient failure")
		}
		return newNopMockExporter(), nil
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)

	p.onBackendChanges([]string{"endpoint-1", "flaky"})
	require.Equal(t, 1, p.NumBackends())
	assertRoutable(t, p, "endpoint-1")

	// Same resolved set again: the failed backend must be retried, not skipped as unchanged.
	p.onBackendChanges([]string{"endpoint-1", "flaky"})

	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, 2, p.NumBackends())
	assertRoutable(t, p, "endpoint-1", "flaky")
}

func TestShutdown_AddInFlight_ShutsDownStragglerExactlyOnce(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	var shutdowns atomic.Int32
	straggler := mockComponent{ShutdownFunc: func(context.Context) error {
		shutdowns.Add(1)
		return nil
	}}

	factory := func(_ context.Context, ep string) (component.Component, error) {
		if ep != endpointWithPort("slow") {
			return newNopMockExporter(), nil
		}
		<-release
		return straggler, nil
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)
	p.exporterAddTimeout = 50 * time.Millisecond
	p.exporterShutdownTimeout = 5 * time.Second
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))

	// "ready" installs immediately so the add timeout applies; with nothing routable
	// awaitExporterBatch would wait "slow" out instead.
	p.onBackendChanges([]string{"ready", "slow"})
	require.Equal(t, 1, p.NumBackends())

	// Release the add while Shutdown is running, so the exporter surfaces after the load balancer
	// has already stopped. It must still be shut down, and never installed.
	time.AfterFunc(50*time.Millisecond, func() { close(release) })
	runWithinTimeout(t, 10*time.Second, func() {
		require.NoError(t, p.Shutdown(t.Context()))
	})

	// No WaitGroup wait here: Shutdown itself must not return with a backend left un-shutdown.
	assert.Equal(t, int32(1), shutdowns.Load())
	assert.Equal(t, 0, p.NumBackends())
}

func TestShutdown_UnboundedAdd_CancelsBeforeResolverShutdown(t *testing.T) {
	ts, tb := getTelemetryAssets(t)

	var once sync.Once
	factoryEntered := make(chan struct{})
	factory := func(ctx context.Context, _ string) (component.Component, error) {
		once.Do(func() { close(factoryEntered) })
		<-ctx.Done()
		return nil, ctx.Err()
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)
	require.Zero(t, p.exporterAddTimeout)

	// Models the async resolvers, whose shutdown takes the same lock their change callbacks run
	// under: it cannot return until the in-flight onBackendChanges does.
	callbackReturned := make(chan struct{})
	p.res = &mockResolver{
		onShutdown: func(context.Context) error {
			<-callbackReturned
			return nil
		},
	}
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))

	go func() {
		p.onBackendChanges([]string{"slow"})
		close(callbackReturned)
	}()
	<-factoryEntered

	// Only addCancel can release that add, so Shutdown has to cancel before shutting the resolver
	// down; the other order deadlocks the two against each other.
	runWithinTimeout(t, 5*time.Second, func() {
		require.NoError(t, p.Shutdown(t.Context()))
	})
}

func TestShutdown_UnboundedAddTimeout_StillBoundedByShutdownTimeout(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	hang := make(chan struct{})
	defer close(hang)

	var once sync.Once
	factoryEntered := make(chan struct{})
	factory := func(context.Context, string) (component.Component, error) {
		once.Do(func() { close(factoryEntered) })
		<-hang // ignores cancellation, like a factory blocked on an unresponsive mount
		return newNopMockExporter(), nil
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)
	require.Zero(t, p.exporterAddTimeout)
	p.exporterShutdownTimeout = 50 * time.Millisecond
	// A resolver that does not resolve on start, so the only add is the one driven below.
	p.res = &mockResolver{}
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))

	go p.onBackendChanges([]string{"slow"})
	<-factoryEntered

	// Shutdown bounds its wait for in-flight adds by exporter_shutdown_timeout rather than by
	// exporter_add_timeout, which is 0 here: taking the add budget would hang the collector.
	runWithinTimeout(t, 5*time.Second, func() {
		_ = p.Shutdown(t.Context())
	})
}

func TestShutdown_HungBackend_BoundedByShutdownTimeout(t *testing.T) {
	ts, tb := getTelemetryAssets(t)

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), func(context.Context, string) (component.Component, error) {
		return &hangingShutdownComponent{}, nil
	}, tb)
	require.NoError(t, err)
	p.exporterShutdownTimeout = 50 * time.Millisecond

	// The DNS, k8s and CloudMap resolvers do not drain the backend set on shutdown the way the
	// static one does, so with those the exporters are still installed when Shutdown reaches them.
	p.res = &mockResolver{
		triggerCallbacks: true,
		onResolve: func(context.Context) ([]string, error) {
			return []string{"hanging"}, nil
		},
	}
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))
	require.Equal(t, 1, p.NumBackends())

	// Deliberately a context with no deadline: that is what otelcol passes to the collector's
	// shutdown, so the bound has to come from exporterShutdownTimeout rather than from the caller.
	runWithinTimeout(t, 5*time.Second, func() {
		_ = p.Shutdown(context.Background()) //nolint:usetesting // see above
	})
}

func TestShutdown_StragglerAfterWaitBegan_IsStillShutDown(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	var shutdowns atomic.Int32
	straggler := mockComponent{ShutdownFunc: func(context.Context) error {
		shutdowns.Add(1)
		return nil
	}}

	factory := func(_ context.Context, ep string) (component.Component, error) {
		if ep != endpointWithPort("slow") {
			return newNopMockExporter(), nil
		}
		<-release
		return straggler, nil
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)
	p.exporterAddTimeout = 10 * time.Millisecond
	p.exporterShutdownTimeout = 50 * time.Millisecond
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))

	p.onBackendChanges([]string{"ready", "slow"})

	// Shutdown gives up on the add after exporterAddTimeout, so the exporter surfaces once
	// nothing is waiting for it anymore. It still has to be shut down rather than leaked.
	require.NoError(t, p.Shutdown(t.Context()))
	close(release)
	p.exportersAddWG.Wait()

	assert.Eventually(t, func() bool {
		return shutdowns.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)
}

// TestShutdown_ConcurrentBackendChanges_IsRaceFree stresses resolver churn against Shutdown. It is
// a probe rather than a proof: it is here to give the race detector something to chew on across
// the lock discipline shared by the ring, exporters, pending and desired.
func TestShutdown_ConcurrentBackendChanges_IsRaceFree(t *testing.T) {
	for range 50 {
		ts, tb := getTelemetryAssets(t)

		p, err := newLoadBalancer(ts.Logger, simpleConfig(), func(context.Context, string) (component.Component, error) {
			return newNopMockExporter(), nil
		}, tb)
		require.NoError(t, err)
		p.exporterShutdownTimeout = 5 * time.Second
		require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))

		var churn sync.WaitGroup
		for i := range 4 {
			churn.Go(func() {
				for j := range 10 {
					p.onBackendChanges([]string{fmt.Sprintf("endpoint-%d-%d", i, j), "shared"})
				}
			})
		}

		require.NoError(t, p.Shutdown(t.Context()))
		churn.Wait()
	}
}

func TestOnBackendChanges_StaleStraggler_DoesNotInstallRemovedEndpoint(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), blockingFactory("b", release, nil), tb)
	require.NoError(t, err)
	p.exporterAddTimeout = 50 * time.Millisecond

	p.onBackendChanges([]string{"a", "b"})
	require.Equal(t, 1, p.NumBackends())

	p.onBackendChanges([]string{"a"})
	require.Equal(t, 1, p.NumBackends())

	close(release)
	p.exportersAddWG.Wait()

	assert.Equal(t, 1, p.NumBackends())
}

func TestOnBackendChanges_SlowAdd_NoDuplicateConcurrentAttempt(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	var calls atomic.Int32

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), blockingFactory("slow", release, &calls), tb)
	require.NoError(t, err)

	p.exporterAddTimeout = 50 * time.Millisecond

	// "ready" keeps a backend installed throughout: awaitExporterBatch only honors the add
	// timeout once something is routable, otherwise it waits rather than emptying the ring.
	p.onBackendChanges([]string{"ready", "slow"})
	p.onBackendChanges([]string{"ready", "slow", "other"})

	close(release)

	p.exportersAddWG.Wait()

	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, 3, p.NumBackends())
}

func TestAwaitExporterBatch_NoBackendReady_ReturnsAsSoonAsOneIs(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	hang := make(chan struct{})
	defer close(hang)

	factory := func(_ context.Context, ep string) (component.Component, error) {
		if ep != endpointWithPort("ready") {
			<-hang
			return newNopMockExporter(), nil
		}
		time.Sleep(200 * time.Millisecond)
		return newNopMockExporter(), nil
	}

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), factory, tb)
	require.NoError(t, err)
	p.exporterAddTimeout = 50 * time.Millisecond

	// Holding the add timeout off while nothing is routable must not turn into waiting for the
	// whole batch: one backend that never starts would otherwise pin onBackendChanges open and
	// keep "ready" out of the ring even after it installs.
	runWithinTimeout(t, 5*time.Second, func() {
		p.onBackendChanges([]string{"ready", "hanging"})
	})

	assert.Equal(t, 1, p.NumBackends())
	assertRoutable(t, p, "ready")
}

func TestOnBackendChanges_FullTierRollover_KeepsOldTierRoutableUntilNewOneStarts(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), blockingFactory("new", release, nil), tb)
	require.NoError(t, err)
	p.exporterAddTimeout = 50 * time.Millisecond

	p.onBackendChanges([]string{"old"})
	require.Equal(t, 1, p.NumBackends())

	// The whole tier is replaced at once, so every installed backend is on its way out. Counting
	// those as "ready" would let the switch complete into an empty ring.
	switched := make(chan struct{})
	go func() {
		p.onBackendChanges([]string{"new"})
		close(switched)
	}()

	// While the new tier is still starting, data has to keep reaching the old one.
	time.Sleep(200 * time.Millisecond)
	assertRoutable(t, p, "old")

	close(release)
	select {
	case <-switched:
	case <-time.After(5 * time.Second):
		t.Fatal("backend switch did not complete")
	}

	assertRoutable(t, p, "new")
}

func TestAwaitExporterBatch_NoBackendReady_WaitsPastAddTimeout(t *testing.T) {
	ts, tb := getTelemetryAssets(t)
	release := make(chan struct{})

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), blockingFactory("slow", release, nil), tb)
	require.NoError(t, err)
	p.exporterAddTimeout = 50 * time.Millisecond

	// Giving up here would leave an empty ring and reject every item, so the add timeout is held
	// off until at least one backend is routable.
	time.AfterFunc(200*time.Millisecond, func() { close(release) })
	p.onBackendChanges([]string{"slow"})

	assert.Equal(t, 1, p.NumBackends())
	assertRoutable(t, p, "slow")
}

func TestRemoveExtraExporters_HungShutdown_GoRoutineTimesOut(t *testing.T) {
	ts, tb := getTelemetryAssets(t)

	p, err := newLoadBalancer(ts.Logger, simpleConfig(), nil, tb)
	require.NoError(t, err)

	p.exporterShutdownTimeout = 50 * time.Millisecond
	p.exporters["hanging"] = newWrappedExporter(&hangingShutdownComponent{}, endpointWithPort("hanging"))

	p.removeExtraExporters()

	runWithinTimeout(t, 100*time.Millisecond, p.exportersShutdownWG.Wait)
}

// hangingShutdownComponent blocks in Shutdown until its context is cancelled.
type hangingShutdownComponent struct {
	component.StartFunc
}

func (*hangingShutdownComponent) Shutdown(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// assertRoutable fails unless every identifier resolves to an installed exporter and the
// endpoints routed to are exactly want. Both halves matter: an endpoint in the ring with no
// exporter fails everything hashing to it, and one missing from the ring silently gets nothing.
func assertRoutable(t *testing.T, lb *loadBalancer, want ...string) {
	t.Helper()

	got := map[string]struct{}{}
	for i := range 1_000 {
		_, endpoint, err := lb.exporterAndEndpoint(fmt.Appendf(nil, "identifier-%d", i))
		require.NoError(t, err)
		got[endpoint] = struct{}{}
	}

	assert.ElementsMatch(t, want, slices.Collect(maps.Keys(got)))
}

// runWithinTimeout fails the test unless fn returns within timeout.
func runWithinTimeout(t *testing.T, timeout time.Duration, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("did not return within %s", timeout)
	}
}

// blockingFactory returns a componentFactory that blocks on release, ignoring ctx entirely,
// before returning a nop exporter for endpoint; any other endpoint returns immediately. calls,
// if non-nil, counts invocations for endpoint.
func blockingFactory(endpoint string, release <-chan struct{}, calls *atomic.Int32) componentFactory {
	endpoint = endpointWithPort(endpoint)

	return func(_ context.Context, ep string) (component.Component, error) {
		if ep != endpoint {
			return newNopMockExporter(), nil
		}

		if calls != nil {
			calls.Add(1)
		}

		<-release

		return newNopMockExporter(), nil
	}
}
