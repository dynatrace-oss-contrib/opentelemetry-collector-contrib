module github.com/open-telemetry/opentelemetry-collector-contrib/receiver/statsdreceiver

go 1.17

require (
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.47.0
	github.com/stretchr/testify v1.7.1
	go.opencensus.io v0.23.0
	go.opentelemetry.io/collector v0.47.1-0.20220323200141-1b7618728835
	go.opentelemetry.io/collector/model v0.47.1-0.20220323200141-1b7618728835
	go.opentelemetry.io/otel v1.6.3
	go.uber.org/multierr v1.8.0
	go.uber.org/zap v1.21.0
	gonum.org/v1/gonum v0.11.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/knadh/koanf v1.4.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	go.opentelemetry.io/otel/metric v0.27.0 // indirect
	go.opentelemetry.io/otel/sdk v1.5.0 // indirect
	go.opentelemetry.io/otel/trace v1.6.3 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	golang.org/x/exp v0.0.0-20200224162631-6cc2880d07d6 // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/open-telemetry/opentelemetry-collector-contrib/internal/common => ../../internal/common
