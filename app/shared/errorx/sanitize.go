package errorx

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

// ErrorLogger is the minimal interface required for logging sanitized errors.
// All go-zero logic structs embed logx.Logger, which satisfies this interface.
type ErrorLogger interface {
	Errorf(format string, v ...any)
}

// SanitizeGRPCError converts a gRPC error into a client-safe error.
//
// Application-level *CodeError values (recognised by the AIM ErrorInfo detail)
// are preserved and returned directly. Raw infrastructure errors — network
// timeouts, gRPC unavailable, driver failures that escaped the service layer —
// are logged with the operation context and replaced by a generic CodeInternal
// error so no infrastructure detail leaks to the client.
func SanitizeGRPCError(logger ErrorLogger, operation string, err error) error {
	if err == nil {
		return nil
	}

	codeErr, deliberate := bizCodeError(err)
	if !deliberate {
		logger.Errorf("rpc %s failed: %v", operation, err)
	}
	return codeErr
}

// SanitizeGRPCErrorNoLog converts a gRPC error into a client-safe error
// without logging. Application-level *CodeError are preserved; infrastructure
// errors are returned as a generic CodeInternal error.
func SanitizeGRPCErrorNoLog(err error) error {
	if err == nil {
		return nil
	}

	codeErr, _ := bizCodeError(err)
	return codeErr
}

// ToGRPCError converts an error raised inside a service into an error that is
// safe to return from a gRPC handler.
//
// *CodeError values pass through untouched, so the exact biz code and message
// reach the caller. Every other error is logged and replaced with a generic
// CodeInternal error. Services use this instead of hand-written per-service
// converters that silently swallow domain codes.
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	if codeErr, ok := unwrapCodeError(err); ok {
		return codeErr
	}

	logx.WithContext(context.Background()).Errorf("unclassified error crossed an RPC boundary: %v", err)
	return NewCodeError(CodeInternal, "internal error")
}

// bizCodeError maps err onto a client-safe *CodeError and reports whether err
// was a deliberate AIM biz error (as opposed to infrastructure noise).
func bizCodeError(err error) (*CodeError, bool) {
	if codeErr, ok := unwrapCodeError(err); ok {
		return codeErr, true
	}

	if HasBizCode(err) {
		return FromGRPCError(err), true
	}

	// A raw gRPC status with a client-visible code (NotFound, InvalidArgument,
	// AlreadyExists, ...) is still a deliberate failure; only infrastructure
	// codes are unknown and must be logged.
	if !IsInfraError(err) {
		if codeErr := FromGRPCError(err); codeErr != nil {
			return codeErr, true
		}
	}

	return NewCodeError(CodeInternal, "internal error"), false
}
