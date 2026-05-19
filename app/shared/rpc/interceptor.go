package rpc

import (
	"context"
	"errors"
	"runtime/debug"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// UnaryErrorInterceptor normalizes transport errors at the RPC boundary.
//
// Business errors expressed as errorx.CodeError are preserved.
// gRPC status errors are converted back to CodeError where possible.
// Any other error is sanitized to internal error so implementation details
// do not leak to RPC clients.
func UnaryErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				panicErr := errorx.NewCodeError(errorx.CodeInternal, "internal error")
				logx.WithContext(ctx).Errorf("panic in %s: %v\n%s", info.FullMethod, r, debug.Stack())
				recordSpanError(ctx, panicErr)
				err = panicErr
				resp = nil
			}
		}()

		resp, err = handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		if codeErr, ok := errors.AsType[*errorx.CodeError](err); ok {
			recordSpanError(ctx, codeErr)
			return resp, codeErr
		}

		if codeErr := errorx.FromGRPCError(err); codeErr != nil {
			recordSpanError(ctx, codeErr)
			if codeErr.Code == errorx.CodeInternal {
				logx.WithContext(ctx).Errorf("rpc error in %s: %v", info.FullMethod, err)
			}

			return nil, codeErr
		}

		internalErr := errorx.NewCodeError(errorx.CodeInternal, "internal error")
		recordSpanError(ctx, err)
		logx.WithContext(ctx).Errorf("rpc error in %s: %v", info.FullMethod, err)

		return nil, internalErr
	}
}

func recordSpanError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	span := trace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
