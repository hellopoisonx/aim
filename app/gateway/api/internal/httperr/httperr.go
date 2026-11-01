// Package httperr owns the gateway's JSON response envelope and the mapping
// from errors to HTTP status codes.
//
// It exists so every HTTP surface of the gateway — go-zero handlers, auth
// middlewares and the WebSocket upgrade handler — serialises failures the same
// way instead of hand-rolling response structs.
package httperr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
	"google.golang.org/grpc/status"
)

// Envelope codes shared with the REST contract.
const (
	CodeOK       = 0
	CodeBadInput = 40000
	CodeInternal = 50000
)

// Envelope is the JSON body every gateway response uses.
type Envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Body any    `json:"body"`
}

// Register installs the gateway response handlers on go-zero's httpx package.
//
// It replaces the former init() side effect in the handler package: the gateway
// entrypoint calls this once while bootstrapping the REST server, so importing
// any handler package no longer mutates process-wide behaviour.
func Register() {
	httpx.SetOkHandler(OkResponse)
	httpx.SetErrorHandlerCtx(ErrorResponse)
}

// OkResponse wraps a successful handler result in the response envelope.
func OkResponse(_ context.Context, v any) any {
	return Envelope{Code: CodeOK, Msg: "ok", Body: v}
}

// ErrorResponse converts an error into an HTTP status and envelope.
func ErrorResponse(ctx context.Context, err error) (int, any) {
	statusCode, envelope := Classify(err)
	if statusCode >= http.StatusInternalServerError {
		logx.WithContext(ctx).Errorf("gateway request failed: %v", err)
	}
	return statusCode, envelope
}

// Classify maps err onto an HTTP status and envelope without writing a response.
//
// Classification order:
//  1. *errorx.CodeError raised in the gateway itself.
//  2. A gRPC status carrying an AIM biz code (or a client-visible gRPC code).
//  3. Remaining plain errors: go-zero's binding layer reports request parse and
//     validation failures as untyped errors, so they map to 400. Every handler
//     must return typed errors for anything else.
func Classify(err error) (int, Envelope) {
	if codeErr, ok := errors.AsType[*errorx.CodeError](err); ok {
		return statusFromCode(codeErr.Code), Envelope{Code: codeErr.Code, Msg: codeErr.Message}
	}

	if statusCode, codeErr, ok := classifyGRPC(err); ok {
		return statusCode, Envelope{Code: codeErr.Code, Msg: codeErr.Message}
	}

	return http.StatusBadRequest, Envelope{Code: CodeBadInput, Msg: err.Error()}
}

// Write serialises err using the gateway envelope. Used by middlewares and the
// WebSocket upgrade path, which answer before a go-zero handler is involved.
func Write(w http.ResponseWriter, err error) {
	statusCode, envelope := Classify(err)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(envelope); encodeErr != nil {
		logx.WithContext(context.Background()).Errorf("gateway: write error envelope failed: %v", encodeErr)
	}
}

// classifyGRPC reports whether err is a gRPC status error and, if so, the
// HTTP status and envelope it maps onto.
func classifyGRPC(err error) (int, *errorx.CodeError, bool) {
	if _, ok := status.FromError(err); !ok {
		return 0, nil, false
	}

	codeErr := errorx.FromGRPCError(err)
	if codeErr == nil {
		return 0, nil, false
	}

	return statusFromCode(codeErr.Code), codeErr, true
}

// statusFromCode derives an HTTP status code from a biz code.
//
// Convention:
//   - 40xxx-59xxx → code / 100, clamped to [400, 511]
//   - Sentinel codes (1001-1006) → explicit semantic mapping
//   - Anything else → 500 (Internal Server Error)
func statusFromCode(code int) int {
	if code >= 40000 && code <= 59999 {
		status := code / 100
		if status < http.StatusBadRequest {
			return http.StatusBadRequest
		}

		if status > http.StatusNetworkAuthenticationRequired {
			return http.StatusInternalServerError
		}

		return status
	}

	switch code {
	case errorx.CodeInvalidCredentials, errorx.CodeTokenInvalid, errorx.CodeTokenExpired:
		return http.StatusUnauthorized
	case errorx.CodeUserNotFound:
		return http.StatusNotFound
	case errorx.CodeUserExists:
		return http.StatusConflict
	case errorx.CodeUserBanned:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
