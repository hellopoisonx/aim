package auth

import (
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

const codeInternal = 50000

type authRPCErrorLogger interface {
	Errorf(format string, v ...any)
}

// sanitizeAuthRPCError converts an RPC error into a client-safe CodeError.
// It preserves the biz code from the gRPC status when available (e.g., 40100
// for auth failures, 40900 for conflicts), while sanitizing internal errors.
// Non-gRPC errors (network timeouts, DNS failures) are logged and mapped
// to a generic internal error.
func sanitizeAuthRPCError(logger authRPCErrorLogger, operation string, err error) error {
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return codeErr
	}

	// Not a gRPC status — network error, client-side failure, etc.
	logger.Errorf("auth rpc %s failed: %v", operation, err)

	return errorx.NewCodeError(codeInternal, "internal error")
}

func (l *LoginLogic) sanitizeAuthRPCError(operation string, err error) error {
	return sanitizeAuthRPCError(l, operation, err)
}

func (l *RegisterLogic) sanitizeAuthRPCError(operation string, err error) error {
	return sanitizeAuthRPCError(l, operation, err)
}

func (l *RefreshLogic) sanitizeAuthRPCError(operation string, err error) error {
	return sanitizeAuthRPCError(l, operation, err)
}

func (l *LogoutLogic) sanitizeAuthRPCError(operation string, err error) error {
	return sanitizeAuthRPCError(l, operation, err)
}
