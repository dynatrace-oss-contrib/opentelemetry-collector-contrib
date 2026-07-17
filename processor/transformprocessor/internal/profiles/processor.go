// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package profiles // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor/internal/profiles"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlprofile"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor/internal/contexts"
)

type Processor struct {
	contexts []contexts.ProfilesConsumer
	logger   *zap.Logger
}

func NewProcessor(contextStatements []contexts.ContextStatements, errorMode ottl.ErrorMode, settings component.TelemetrySettings, profileFunctions map[string]ottl.Factory[*ottlprofile.TransformContext]) (*Processor, error) {
	pc, err := contexts.NewProfileParserCollection(settings, contexts.WithProfileParser(profileFunctions), contexts.WithProfileErrorMode(errorMode))
	if err != nil {
		return nil, err
	}

	contexts := make([]contexts.ProfilesConsumer, len(contextStatements))
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

func (p *Processor) ProcessProfiles(ctx context.Context, ld pprofile.Profiles) (pprofile.Profiles, error) {
	for _, c := range p.contexts {
		err := c.ConsumeProfiles(ctx, ld)
		if err != nil {
			p.logger.Error("failed processing profiles", zap.Error(err))
			return ld, err
		}
	}
	return ld, nil
}
