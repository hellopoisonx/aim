// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package conversations

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/logic/conversations"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func TransferGroupOwnerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.TransferGroupOwnerRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := conversations.NewTransferGroupOwnerLogic(r.Context(), svcCtx)
		resp, err := l.TransferGroupOwner(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
