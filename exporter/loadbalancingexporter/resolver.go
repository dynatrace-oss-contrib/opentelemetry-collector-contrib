// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter // import "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter"

import (
	"context"

	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter/internal/metadata"
)

// resolver determines the contract for sources of backend endpoint information
type resolver interface {
	// resolve returns the current list of endpoints.
	// returns either a non-nil error and a nil list of endpoints, or a non-nil list of endpoints and nil error.
	resolve(context.Context) ([]string, error)

	// start signals the resolver to start its work
	start(context.Context) error

	// shutdown signals the resolver to finish its work. This should block until the current resolutions are finished.
	// Once this is invoked, callbacks will not be triggered anymore and will need to be registered again in case the consumer
	// decides to restart the resolver.
	shutdown(context.Context) error

	// onChange registers a function to call back whenever the list of backends is updated.
	// Make sure to register the callbacks before starting the exporter.
	onChange(func([]string))
}

// newResolver builds the one resolver configured in cfg. Exactly one has to be set.
func newResolver(logger *zap.Logger, cfg ResolverSettings, telemetry *metadata.TelemetryBuilder) (resolver, error) {
	count := 0
	for _, configured := range []bool{
		cfg.Static.HasValue(),
		cfg.DNS.HasValue(),
		cfg.K8sSvc.HasValue(),
		cfg.AWSCloudMap.HasValue(),
	} {
		if configured {
			count++
		}
	}
	if count > 1 {
		return nil, errMultipleResolversProvided
	}

	switch {
	case cfg.Static.HasValue():
		return newStaticResolver(cfg.Static.Get().Hostnames, telemetry)

	case cfg.DNS.HasValue():
		dns := cfg.DNS.Get()
		return newDNSResolver(
			logger.With(zap.String("resolver", "dns")),
			dns.Hostname,
			dns.Port,
			dns.Interval,
			dns.Timeout,
			telemetry,
		)

	case cfg.K8sSvc.HasValue():
		k8s := cfg.K8sSvc.Get()
		return newK8sResolver(
			// A nil client: it is created during start() so the config can be validated from
			// outside a k8s cluster.
			nil,
			logger.With(zap.String("resolver", "k8s service")),
			k8s.Service,
			k8s.Ports,
			k8s.Timeout,
			k8s.ReturnHostnames,
			telemetry,
		)

	case cfg.AWSCloudMap.HasValue():
		aws := cfg.AWSCloudMap.Get()
		return newCloudMapResolver(
			logger.With(zap.String("resolver", "aws_cloud_map")),
			&aws.NamespaceName,
			&aws.ServiceName,
			aws.Port,
			&aws.HealthStatus,
			aws.Interval,
			aws.Timeout,
			aws.OwnerAccount,
			telemetry,
		)
	}

	return nil, errNoResolver
}
