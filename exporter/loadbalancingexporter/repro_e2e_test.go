// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package loadbalancingexporter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

const reproYAMLTmpl = `
routing_key: service
protocol:
  otlp:
    tls:
      insecure: true
    sending_queue:
      enabled: false
    retry_on_failure:
      enabled: false
resolver:
  static:
    hostnames: %s
`

// TestReproE2ELogsShutdownHang runs the exporter from a YAML config through the real factory and
// static resolver. The OTLP sub-exporter for "bad%zz" fails to start, so the endpoint is in the
// ring without an exporter. Sub-exporter queue and retry are disabled so the healthy backend
// fails fast and only the leaked consumeWG count can block Shutdown.
func TestReproE2ELogsShutdownHang(t *testing.T) {
	for name, hosts := range map[string]string{"control": `[localhost:4317]`, "bad": `[localhost:4317, "bad%zz"]`} {
		t.Run(name, func(t *testing.T) { runE2E(t, fmt.Sprintf(reproYAMLTmpl, hosts)) })
	}
}

func runE2E(t *testing.T, reproYAML string) {
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(reproYAML), &raw))
	f := NewFactory()
	cfg := f.CreateDefaultConfig()
	require.NoError(t, confmap.NewFromStringMap(raw).Unmarshal(cfg))

	set := exportertest.NewNopSettings(f.Type())
	set.Logger = zaptest.NewLogger(t)
	exp, err := f.CreateLogs(t.Context(), set, cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(t.Context(), componenttest.NewNopHost()))

	var errs int
	for range 20 {
		ld := plog.NewLogs()
		for i := range 50 {
			simpleLogWithServiceName(fmt.Sprintf("svc-%d", i)).ResourceLogs().MoveAndAppendTo(ld.ResourceLogs())
		}
		if err := exp.ConsumeLogs(t.Context(), ld); err != nil {
			errs++
			if errs == 1 {
				t.Logf("ConsumeLogs error: %v", err)
			}
		}
	}
	t.Logf("%d/20 batches failed", errs)

	done := make(chan error, 1)
	go func() { done <- exp.Shutdown(context.Background()) }()
	select {
	case err := <-done:
		t.Logf("Shutdown returned: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("collector Shutdown hangs")
	}
}
