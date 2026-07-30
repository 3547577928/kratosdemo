package server

import (
	kmetrics "github.com/go-kratos/kratos/contrib/otel/v3/metrics"
	prom "github.com/prometheus/client_golang/prometheus"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const meterName = "testdemo"

var (
	metricRequests metric.Int64Counter
	metricSeconds  metric.Float64Histogram
	metricGatherer prom.Gatherer
)

func NewMeterProvider() metric.MeterProvider {
	registry := prom.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		panic(err)
	}
	metricGatherer = prom.Gatherers{prom.DefaultGatherer, registry}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter(meterName)

	metricRequests, err = kmetrics.DefaultRequestsCounter(meter, kmetrics.DefaultServerRequestsCounterName)
	if err != nil {
		panic(err)
	}
	metricSeconds, err = kmetrics.DefaultSecondsHistogram(meter, kmetrics.DefaultServerSecondsHistogramName)
	if err != nil {
		panic(err)
	}

	return provider
}
