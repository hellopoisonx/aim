package errorx

// ErrorLogger is the minimal interface required for logging sanitized errors.
// All go-zero logic structs embed logx.Logger, which satisfies this interface.
type ErrorLogger interface {
	Errorf(format string, v ...any)
}

// SanitizeGRPCError converts a gRPC error into a client-safe error.
//
// Application-level *CodeError extracted via FromGRPCError are preserved and
// returned directly. Infrastructure errors (network timeout, gRPC unavailable,
// etc.) are logged via the provided logger with the operation context and
// returned as a generic CodeInternal error ("internal error") to prevent
// infrastructure details from leaking to the client.
func SanitizeGRPCError(logger ErrorLogger, operation string, err error) error {
	if codeErr := FromGRPCError(err); codeErr != nil {
		return codeErr
	}

	logger.Errorf("rpc %s failed: %v", operation, err)
	return NewCodeError(CodeInternal, "internal error")
}

// SanitizeGRPCErrorNoLog converts a gRPC error into a client-safe error
// without logging. Application-level *CodeError are preserved; infrastructure
// errors are returned as a generic CodeInternal error.
func SanitizeGRPCErrorNoLog(err error) error {
	if codeErr := FromGRPCError(err); codeErr != nil {
		return codeErr
	}
	return NewCodeError(CodeInternal, "internal error")
}
