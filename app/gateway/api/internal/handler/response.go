package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/zeromicro/go-zero/rest/httpx"
	"google.golang.org/grpc/status"
)

const (
	codeOK       = 0
	codeBadInput = 40000
	codeInternal = 50000
)

type responseEnvelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Body any    `json:"body"`
}

func init() {
	httpx.SetOkHandler(okResponse)
	httpx.SetErrorHandlerCtx(errorResponse)
}

func okResponse(_ context.Context, v any) any {
	return responseEnvelope{Code: codeOK, Msg: "ok", Body: v}
}

func errorResponse(_ context.Context, err error) (int, any) {
	// 1. Try CodeError first (most common path: gateway-issued or unwrapped from RPC).
	if codeErr, ok := errors.AsType[*errorx.CodeError](err); ok {
		return httpStatusFromCode(codeErr.Code), responseEnvelope{Code: codeErr.Code, Msg: codeErr.Message}
	}

	// 2. Try extracting CodeError from gRPC status.
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return httpStatusFromCode(codeErr.Code), responseEnvelope{Code: codeErr.Code, Msg: codeErr.Message}
	}

	// 3. Raw gRPC status that FromGRPCError couldn't decode — sanitize.
	if _, ok := status.FromError(err); ok {
		return http.StatusInternalServerError, responseEnvelope{Code: codeInternal, Msg: "internal error"}
	}

	// 4. Plain error (e.g., validation from httpx.Parse).
	return http.StatusBadRequest, responseEnvelope{Code: codeBadInput, Msg: err.Error()}
}

// httpStatusFromCode derives an HTTP status code from a biz code.
//
// Convention:
//   - 40xxx-59xxx → code / 100, clamped to [400, 511]
//   - Sentinel codes (1001-1006) → explicit semantic mapping
//   - Anything else → 500 (Internal Server Error)
func httpStatusFromCode(code int) int {
	if code >= 40000 && code <= 59999 {
		status := code / 100
		// Clamp to valid HTTP status range.
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
