// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter"

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter/internal/metadata"
)

const (
	defaultPort                    = "4317"
	defaultExporterAddTimeout      = 5 * time.Second
	defaultExporterShutdownTimeout = 30 * time.Second
)

var (
	errNoResolver                = errors.New("no resolvers specified for the exporter")
	errMultipleResolversProvided = errors.New("only one resolver should be specified")
)

type componentFactory func(ctx context.Context, endpoint string) (component.Component, error)

// lifecyclePhase tracks how far Shutdown has progressed. It only moves forward.
type lifecyclePhase int

const (
	// phaseRunning installs new exporters normally.
	phaseRunning lifecyclePhase = iota
	// phaseStopping means Shutdown has begun, so an exporter that finishes starting from here on
	// is shut down instead of installed.
	phaseStopping
	// phaseDraining means Shutdown is already waiting on exportersShutdownWG and nothing more may
	// be added to it; see shutdownExporterAsync.
	phaseDraining
)

// loadBalancer keeps a set of backend exporters in sync with a resolver and routes data across
// them through a consistent hash ring.
//
// Endpoints are keyed everywhere on the string the resolver produced. endpointWithPort is applied
// only when building the underlying exporter, so the ring, the exporter map and the desired set
// all share one key space.
type loadBalancer struct {
	logger           *zap.Logger
	host             component.Host
	res              resolver
	componentFactory componentFactory

	// Immutable after construction. Zero means unbounded.
	exporterAddTimeout      time.Duration
	exporterShutdownTimeout time.Duration

	// addLifetimeCtx is passed to every exporter start. Shutdown cancels it to release the starts
	// that are still in flight.
	addLifetimeCtx    context.Context
	addLifetimeCancel context.CancelFunc

	exportersAddWG      sync.WaitGroup
	exportersShutdownWG sync.WaitGroup

	// updateLock guards every field below it.
	updateLock sync.RWMutex
	ring       *hashRing
	exporters  map[string]*wrappedExporter
	// desired is the endpoint set from the most recent resolution. pending is the subset whose
	// exporter is still starting, which is what keeps overlapping resolutions from starting the
	// same endpoint twice.
	desired map[string]struct{}
	pending map[string]struct{}
	phase   lifecyclePhase
}

// Create new load balancer
func newLoadBalancer(logger *zap.Logger, cfg component.Config, factory componentFactory, telemetry *metadata.TelemetryBuilder) (*loadBalancer, error) {
	oCfg := cfg.(*Config)

	res, err := newResolver(logger, oCfg.Resolver, telemetry)
	if err != nil {
		return nil, err
	}

	addCtx, addCancel := context.WithCancel(context.Background())

	return &loadBalancer{
		logger:                  logger,
		res:                     res,
		componentFactory:        factory,
		exporters:               map[string]*wrappedExporter{},
		desired:                 map[string]struct{}{},
		pending:                 map[string]struct{}{},
		addLifetimeCtx:          addCtx,
		addLifetimeCancel:       addCancel,
		exporterAddTimeout:      oCfg.ExporterAddTimeout,
		exporterShutdownTimeout: oCfg.ExporterShutdownTimeout,
	}, nil
}

func (lb *loadBalancer) Start(ctx context.Context, host component.Host) error {
	lb.res.onChange(lb.onBackendChanges)
	lb.host = host

	return lb.res.start(ctx)
}

// onBackendChanges is the only entry point for topology changes. It runs on the resolver's
// goroutine and drives one sequence:
//
//  1. record the new desired set and launch an exporter start for each endpoint not installed yet
//  2. wait up to exporterAddTimeout for those starts, but never past the point where nothing is
//     routable, because step 3 would then rebuild an empty ring
//  3. drop the endpoints that are no longer desired and rebuild the ring
//
// Steps 1 and 3 hold updateLock; step 2 deliberately does not, so exports keep flowing while new
// backends start. A start that misses the step 2 deadline installs itself whenever it finishes.
func (lb *loadBalancer) onBackendChanges(resolved []string) {
	batch := lb.launchExporterBatch(resolved)
	lb.awaitExporterBatch(batch)

	lb.updateLock.Lock()
	defer lb.updateLock.Unlock()

	lb.removeExtraExporters()
	lb.rebuildRingLocked()
}

// addBatch tracks the exporter starts launched by one launchExporterBatch call. waiterGone is
// closed once awaitExporterBatch stops waiting on them, which is how a straggler learns that it
// has to rebuild the ring itself.
type addBatch struct {
	done       chan struct{}
	waiterGone chan struct{}
	started    int
}

// launchExporterBatch records endpoints as the desired set and starts an exporter for each one
// that is neither installed nor already starting.
//
// The goroutines are launched while holding updateLock rather than after releasing it, which
// orders every exportersAddWG.Go against the phase change Shutdown makes under the same lock.
// That is what lets Shutdown wait on that WaitGroup without racing an Add.
func (lb *loadBalancer) launchExporterBatch(endpoints []string) addBatch {
	lb.updateLock.Lock()
	defer lb.updateLock.Unlock()

	if lb.phase != phaseRunning {
		return addBatch{}
	}

	lb.desired = make(map[string]struct{}, len(endpoints))
	missing := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		lb.desired[endpoint] = struct{}{}

		_, installed := lb.exporters[endpoint]
		_, starting := lb.pending[endpoint]
		if !installed && !starting {
			lb.pending[endpoint] = struct{}{}
			missing = append(missing, endpoint)
		}
	}

	batch := addBatch{
		// Buffered so a straggler's send never blocks after awaitExporterBatch stops receiving.
		done:       make(chan struct{}, len(missing)),
		waiterGone: make(chan struct{}),
		started:    len(missing),
	}

	for _, endpoint := range missing {
		lb.exportersAddWG.Go(func() {
			lb.createAndInstallExporter(endpoint, batch.waiterGone)
			batch.done <- struct{}{}
		})
	}

	return batch
}

// awaitExporterBatch waits for batch to finish starting, holding no lock so that concurrent
// ConsumeTraces/Metrics/Logs calls are never blocked. Anything still in flight when it returns
// keeps running and installs itself later; see createAndInstallExporter.
func (lb *loadBalancer) awaitExporterBatch(batch addBatch) {
	// Also covers the zero-value batch from a stopped load balancer, whose channels are nil.
	if batch.started == 0 {
		return
	}

	defer close(batch.waiterGone)

	// nil while unbounded, and set back to nil once fired so it is never selected twice
	var deadline <-chan time.Time
	if lb.exporterAddTimeout > 0 {
		timer := time.NewTimer(lb.exporterAddTimeout)
		defer timer.Stop()
		deadline = timer.C
	}

	var expired bool
	for remaining := batch.started; remaining > 0; {
		// Once the deadline has passed, the first backend to become routable releases the wait,
		// so one backend that never starts cannot keep the rest out of the ring.
		if expired && lb.numRoutable() > 0 {
			lb.logger.Warn("timed out waiting for new backends to start; routing to the backends that are ready",
				zap.Duration("timeout", lb.exporterAddTimeout))

			return
		}

		select {
		case <-batch.done:
			remaining--
		case <-lb.addLifetimeCtx.Done():
			return
		case <-deadline:
			expired, deadline = true, nil
			if lb.numRoutable() == 0 {
				lb.logger.Warn("no backend is ready yet; waiting past the add timeout rather than dropping data",
					zap.Duration("timeout", lb.exporterAddTimeout))
			}
		}
	}
}

// numRoutable counts the installed exporters that the current resolution still wants. NumBackends
// would also count the ones removeExtraExporters is about to drop -- on a full-tier rollover, all
// of them -- and returning while only those are ready would rebuild an empty ring.
func (lb *loadBalancer) numRoutable() int {
	lb.updateLock.RLock()
	defer lb.updateLock.RUnlock()

	routable := 0
	for endpoint := range lb.exporters {
		if _, wanted := lb.desired[endpoint]; wanted {
			routable++
		}
	}

	return routable
}

// createAndInstallExporter creates and starts the exporter for endpoint, then installs it under a
// short-lived lock. It runs under lb.addLifetimeCtx, so it may outlive the onBackendChanges call
// that spawned it and install itself whenever it eventually finishes.
func (lb *loadBalancer) createAndInstallExporter(endpoint string, waiterGone <-chan struct{}) {
	exp, err := lb.componentFactory(lb.addLifetimeCtx, endpointWithPort(endpoint))
	if err != nil {
		lb.logger.Error("failed to create new exporter for endpoint", zap.String("endpoint", endpoint), zap.Error(err))
		// nothing to shut down; release the endpoint for another attempt
		lb.updateLock.Lock()
		delete(lb.pending, endpoint)
		lb.updateLock.Unlock()

		return
	}

	we := newWrappedExporter(exp, endpointWithPort(endpoint))

	startErr := we.Start(lb.addLifetimeCtx, lb.host)
	if startErr != nil {
		lb.logger.Error("failed to start new exporter for endpoint", zap.String("endpoint", endpoint), zap.Error(startErr))
	}

	lb.updateLock.Lock()
	defer lb.updateLock.Unlock()
	delete(lb.pending, endpoint)

	_, wanted := lb.desired[endpoint]
	if startErr != nil || lb.phase != phaseRunning || !wanted {
		lb.shutdownExporterAsync(we)
		return
	}

	lb.exporters[endpoint] = we

	// Only a straggler rebuilds the ring; a waiter still on this batch rebuilds once for all of
	// it, and doing both would cost a full sort per backend under the write lock. Reading
	// waiterGone under the lock is what makes the handover safe: an attempt that installs after
	// the waiter's rebuild is guaranteed to see it closed.
	if closed(waiterGone) {
		lb.rebuildRingLocked()
	}
}

// removeExtraExporters drops the installed exporters that are no longer desired. Must be called
// with updateLock held for writing; callers are responsible for rebuilding the ring afterward.
func (lb *loadBalancer) removeExtraExporters() {
	for endpoint, we := range lb.exporters {
		if _, wanted := lb.desired[endpoint]; !wanted {
			lb.shutdownExporterAsync(we)
			delete(lb.exporters, endpoint)
		}
	}
}

// rebuildRingLocked rebuilds the ring from the installed exporters. Endpoints without one are
// left out on purpose: keeping one in fails every identifier hashing to it instead of spreading
// them over the backends that are ready. Must be called with updateLock held for writing.
func (lb *loadBalancer) rebuildRingLocked() {
	newRing := newHashRing(slices.Collect(maps.Keys(lb.exporters)))
	if !newRing.equal(lb.ring) {
		lb.ring = newRing
	}
}

// exporterAndEndpoint returns the exporter and the endpoint for the given identifier.
func (lb *loadBalancer) exporterAndEndpoint(identifier []byte) (*wrappedExporter, string, error) {
	// NOTE: make rolling updates of next tier of collectors work. currently, this may cause
	// data loss because the latest batches sent to outdated backend will never find their way out.
	// for details: https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/1690
	lb.updateLock.RLock()
	defer lb.updateLock.RUnlock()

	endpoint := lb.ring.endpointFor(identifier)

	exp, found := lb.exporters[endpoint]
	if !found {
		// something is really wrong... how come we couldn't find the exporter??
		return nil, "", fmt.Errorf("couldn't find the exporter for the endpoint %q", endpoint)
	}

	return exp, endpoint, nil
}

// NumBackends returns the current number of resolved backend exporters.
func (lb *loadBalancer) NumBackends() int {
	lb.updateLock.RLock()
	defer lb.updateLock.RUnlock()

	return len(lb.exporters)
}

// Shutdown stops resolving and tears down every backend exporter, in four steps:
//
//  1. release the in-flight adds, then stop the resolver
//  2. close the load balancer to new installs
//  3. let the canceled adds settle, so nothing is still on its way into exportersShutdownWG
//  4. hand the installed exporters to that WaitGroup, close it, and wait for the drain
//
// Steps 1 and 2 are ordered: every async resolver invokes its change callbacks under a read lock
// that its own shutdown then takes for writing, so res.shutdown blocks behind an in-flight
// onBackendChanges that only addLifetimeCancel can release.
func (lb *loadBalancer) Shutdown(ctx context.Context) error {
	lb.addLifetimeCancel()

	err := lb.res.shutdown(ctx)

	lb.updateLock.Lock()
	lb.phase = phaseStopping
	lb.updateLock.Unlock()

	// From phaseStopping on, an add that finishes hands its exporter to shutdownExporterAsync
	// instead of installing it. Bounded by the shutdown budget rather than the add budget: a
	// factory that ignores cancellation would otherwise hang the collector's shutdown.
	waitWithTimeout(&lb.exportersAddWG, lb.exporterShutdownTimeout)

	// Registering the installed exporters and closing the gate under one lock is what makes the
	// wait below safe: no further exportersShutdownWG.Go can happen once the phase is draining, so
	// the last Add happens-before the first Wait. An add still stuck past step 3 is detached
	// instead.
	lb.updateLock.Lock()
	for _, we := range lb.exporters {
		lb.shutdownExporterAsync(we)
	}

	lb.exporters = map[string]*wrappedExporter{}
	lb.rebuildRingLocked()
	lb.phase = phaseDraining
	lb.updateLock.Unlock()

	// The installed exporters go through shutdownExporterAsync too, so they are bounded by
	// exporterShutdownTimeout rather than by the caller's context, which otelcol supplies without
	// a deadline. Their errors are logged, not returned: the bounded wait may return first.
	waitWithTimeout(&lb.exportersShutdownWG, lb.exporterShutdownTimeout)

	return err
}

// shutdownExporterAsync shuts we down on its own goroutine, so a slow backend never holds
// updateLock. Must be called with updateLock held for writing.
func (lb *loadBalancer) shutdownExporterAsync(we *wrappedExporter) {
	shutdown := func() {
		shutdownCtx := context.Background()
		if lb.exporterShutdownTimeout > 0 {
			var cancel context.CancelFunc
			shutdownCtx, cancel = context.WithTimeout(shutdownCtx, lb.exporterShutdownTimeout)
			defer cancel()
		}

		if err := we.Shutdown(shutdownCtx); err != nil {
			lb.logger.Warn("failed to shut down backend exporter", zap.Error(err))
		}
	}

	if lb.phase == phaseDraining {
		// adding to exportersShutdownWG once Shutdown is waiting on it would be a reuse race, and
		// nothing is left to wait for this one anyway
		lb.logger.Warn("shutting down a backend exporter that arrived after the shutdown wait began")
		go shutdown()

		return
	}

	lb.exportersShutdownWG.Go(shutdown)
}

// waitWithTimeout waits for wg to complete, giving up after timeout elapses. A non-positive
// timeout waits indefinitely.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	if timeout <= 0 {
		wg.Wait()
		return
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
	}
}

func endpointWithPort(endpoint string) string {
	if !strings.Contains(endpoint, ":") {
		endpoint = fmt.Sprintf("%s:%s", endpoint, defaultPort)
	}
	return endpoint
}

// closed reports whether ch has been closed, without blocking on it.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
