package server

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// InitTracer 初始化 OpenTelemetry 链路追踪
func InitTracer(url string, serviceName string) error {
	// 创建 OTLP HTTP 导出器
	exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpoint(url), otlptracehttp.WithInsecure())
	if err != nil {
		return err
	}

	// 创建 TracerProvider
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()), // 采样率，生产环境建议调整
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)
	return nil
}
