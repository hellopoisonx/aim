package errorx

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error codes for auth domain sentinel errors.
// These are defined here alongside the RPC-level codes to provide a single catalog.
const (
	CodeInvalidCredentials = 1001
	CodeUserNotFound       = 1002
	CodeUserExists         = 1003
	CodeTokenInvalid       = 1004
	CodeTokenExpired       = 1005
	CodeUserBanned         = 1006
)

// Bot OpenAPI error codes (40110-40130). They live in the 401xx range so the
// responseEnvelope mapper drives a 401 HTTP status by default.
const (
	// CodeBotTokenInvalid — `Authorization: Bot <token>` missing/malformed/unknown.
	CodeBotTokenInvalid = 40110
	// CodeBotTokenRevoked — token is revoked or expired.
	CodeBotTokenRevoked = 40111
	// CodeBotDisabled — owner user_info.status indicates the bot identity is disabled.
	CodeBotDisabled = 40112
	// CodeBotScopeDenied — token does not carry the required scope (e.g. messages:send).
	CodeBotScopeDenied = 40310
	// CodeBotWebhookInvalid — webhook URL or events list failed validation.
	CodeBotWebhookInvalid = 40010
)

// RPC-level error code ranges (service-side).
// Category = code / 100 maps to HTTP status:
//
//	40xxx → 400, 401xx → 401, 403xx → 403, 404xx → 404,
//	409xx → 409, 429xx → 429, 50xxx → 500.
const (
	CodeBadInput  = 40000
	CodeAuth      = 40100
	CodeForbidden = 40300
	CodeNotFound  = 40400
	CodeConflict  = 40900
	CodeRateLimit = 42900
	CodeInternal  = 50000
)

var (
	ErrInvalidCredentials = NewCodeError(CodeInvalidCredentials, "invalid email or password")
	ErrUserNotFound       = NewCodeError(CodeUserNotFound, "user not found")
	ErrUserExists         = NewCodeError(CodeUserExists, "user already exists")
	ErrTokenInvalid       = NewCodeError(CodeTokenInvalid, "invalid token")
	ErrTokenExpired       = NewCodeError(CodeTokenExpired, "token expired")
	ErrUserBanned         = NewCodeError(CodeUserBanned, "user is banned")
)

// CodeError represents an error with a biz code and message.
// It implements GRPCStatus() so gRPC carries the proper status code
// when the error crosses a gRPC boundary.
type CodeError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *CodeError) Error() string {
	return e.Message
}

// GRPCStatus returns the gRPC status for this error.
// When a gRPC server handler returns a *CodeError, the gRPC framework
// calls this method to derive the wire status — the biz code category
// maps to a gRPC code and the message is preserved.
func (e *CodeError) GRPCStatus() *status.Status {
	return status.New(grpcCodeFromBizCode(e.Code), e.Message)
}

// NewCodeError creates a new CodeError.
func NewCodeError(code int, message string) *CodeError {
	return &CodeError{
		Code:    code,
		Message: message,
	}
}

// NewCodeErrorf creates a new CodeError with formatted message.
func NewCodeErrorf(code int, format string, args ...any) *CodeError {
	return &CodeError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// Is checks if the error matches the target by comparing biz codes.
func (e *CodeError) Is(target error) bool {
	if ce, ok := errors.AsType[*CodeError](target); ok {
		return e.Code == ce.Code
	}

	return false
}

// FromGRPCError extracts a CodeError from a gRPC error.
// It recovers the biz code from the gRPC status code and preserves the message.
// Returns nil if err is nil or not a gRPC status error.
//
// Infrastructure gRPC codes (Internal, Unavailable, DataLoss, Aborted,
// DeadlineExceeded, Unknown) are sanitized: their original message is replaced
// with "internal error" to prevent infrastructure details from leaking.
func FromGRPCError(err error) *CodeError {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return nil
	}

	code := bizCodeFromGRPCCode(st.Code())
	msg := st.Message()

	if isInfraGRPCCode(st.Code()) {
		msg = "internal error"
	}

	return NewCodeError(code, msg)
}

// isInfraGRPCCode returns true for gRPC codes that describe infrastructure
// conditions rather than application-level errors.
func isInfraGRPCCode(c codes.Code) bool {
	switch c {
	case codes.Internal, codes.Unavailable, codes.DataLoss,
		codes.Aborted, codes.DeadlineExceeded, codes.Unknown:
		return true
	default:
		return false
	}
}

// grpcCodeFromBizCode maps a biz code to the closest gRPC status code.
// Sentinel codes (1xxx) always map to Unauthenticated since they are auth-related.
func grpcCodeFromBizCode(code int) codes.Code {
	category := code / 100

	switch {
	case category == 400:
		return codes.InvalidArgument
	case category == 401:
		return codes.Unauthenticated
	case category == 403:
		return codes.PermissionDenied
	case category == 404:
		return codes.NotFound
	case category == 409:
		return codes.AlreadyExists
	case category == 429:
		return codes.ResourceExhausted
	case category >= 500:
		return codes.Internal
	default:
		// Sentinel codes (1001-1006) and unknown codes.
		return codes.Unauthenticated
	}
}

// bizCodeFromGRPCCode maps a gRPC status code back to a biz code.
// The returned biz code is a category-wide code (e.g., 40000, 40100);
// the exact sub-category code from the server is not recoverable from
// the gRPC status alone. If higher fidelity is needed in the future,
// embed the exact biz code in gRPC status details.
func bizCodeFromGRPCCode(c codes.Code) int {
	switch c {
	case codes.InvalidArgument:
		return 40000
	case codes.Unauthenticated:
		return 40100
	case codes.PermissionDenied:
		return 40300
	case codes.NotFound:
		return 40400
	case codes.AlreadyExists:
		return 40900
	case codes.ResourceExhausted:
		return 42900
	case codes.Internal:
		return 50000
	default:
		return 50000
	}
}
