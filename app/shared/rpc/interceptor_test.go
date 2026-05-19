package rpc

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryErrorInterceptor_PreservesCodeError(t *testing.T) {
	t.Parallel()

	interceptor := UnaryErrorInterceptor()
	orig := errorx.NewCodeError(errorx.CodeBadInput, "missing field")

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "resp", orig
	})
	require.Equal(t, "resp", resp)
	require.Same(t, orig, err)
}

func TestUnaryErrorInterceptor_PreservesNilError(t *testing.T) {
	t.Parallel()

	interceptor := UnaryErrorInterceptor()
	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "resp", nil
	})
	require.Equal(t, "resp", resp)
	require.NoError(t, err)
}

func TestUnaryErrorInterceptor_ConvertsGrpcStatusToCodeError(t *testing.T) {
	t.Parallel()

	interceptor := UnaryErrorInterceptor()
	grpcErr := status.Error(codes.InvalidArgument, "bad input")

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, grpcErr
	})
	require.Nil(t, resp)
	require.NotNil(t, err)

	codeErr, ok := errors.AsType[*errorx.CodeError](err)
	require.True(t, ok)
	require.Equal(t, errorx.CodeBadInput, codeErr.Code)
	require.Equal(t, "bad input", codeErr.Message)
}

func TestUnaryErrorInterceptor_SanitizesGrpcInfraStatus(t *testing.T) {
	t.Parallel()

	interceptor := UnaryErrorInterceptor()
	grpcErr := status.Error(codes.Internal, "db connection pool exhausted")

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, grpcErr
	})
	require.Nil(t, resp)

	codeErr, ok := errors.AsType[*errorx.CodeError](err)
	require.True(t, ok)
	require.Equal(t, errorx.CodeInternal, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}

func TestUnaryErrorInterceptor_SanitizesPlainError(t *testing.T) {
	t.Parallel()

	interceptor := UnaryErrorInterceptor()
	plainErr := errors.New("pgx: connection reset by peer")

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, plainErr
	})
	require.Nil(t, resp)

	codeErr, ok := errors.AsType[*errorx.CodeError](err)
	require.True(t, ok)
	require.Equal(t, errorx.CodeInternal, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}

func TestUnaryErrorInterceptor_RecordsSpanError(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(spans))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	interceptor := UnaryErrorInterceptor()
	plainErr := errors.New("unexpected failure")

	ctx, span := otel.Tracer("test").Start(context.Background(), "test-op")
	defer span.End()

	_, err := interceptor(ctx, "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return nil, plainErr
	})
	require.Error(t, err)
	span.End()

	ended := spans.Ended()
	require.NotEmpty(t, ended)
	require.NotEmpty(t, ended[0].Events())
}

func TestUnaryErrorInterceptor_RecoversPanic(t *testing.T) {
	t.Parallel()

	interceptor := UnaryErrorInterceptor()

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/test.Panic"}, func(ctx context.Context, req any) (any, error) {
		panic("test panic")
	})
	require.Nil(t, resp)

	codeErr, ok := errors.AsType[*errorx.CodeError](err)
	require.True(t, ok)
	require.Equal(t, errorx.CodeInternal, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}
