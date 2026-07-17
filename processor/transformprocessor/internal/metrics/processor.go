// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package metrics // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor/internal/metrics"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottldatapoint"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlexemplar"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlmetric"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor/internal/contexts"
)

type Processor struct {
	contexts []contexts.MetricsConsumer
	logger   *zap.Logger
}

func NewProcessor(contextStatements []contexts.ContextStatements, errorMode ottl.ErrorMode, settings component.TelemetrySettings, metricFunctions map[string]ottl.Factory[*ottlmetric.TransformContext], dataPointFunctions map[string]ottl.Factory[*ottldatapoint.TransformContext], exemplarFunctions map[string]ottl.Factory[*ottlexemplar.TransformContext]) (*Processor, error) {
	pc, err := contexts.NewMetricParserCollection(settings, contexts.WithMetricParser(metricFunctions), contexts.WithDataPointParser(dataPointFunctions), contexts.WithExemplarParser(exemplarFunctions), contexts.WithMetricErrorMode(errorMode))
	if err != nil {
		return nil, err
	}

	contexts := make([]contexts.MetricsConsumer, len(contextStatements))
	var errors error
	for i, cs := range contextStatements {
		context, err := pc.ParseContextStatements(cs)
		if err != nil {
			errors = multierr.Append(errors, err)
		}
		contexts[i] = context
	}

	if errors != nil {
		return nil, errors
	}

	return &Processor{
		contexts: contexts,
		logger:   settings.Logger,
	}, nil
}

func (p *Processor) ProcessMetrics(ctx context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	for _, c := range p.contexts {
		err := c.ConsumeMetrics(ctx, md)
		if err != nil {
			p.logger.Error("failed processing metrics", zap.Error(err))
			return md, err
		}
	}
	return md, nil
}
