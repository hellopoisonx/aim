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

func GetConversationHistoryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetConversationHistoryRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := conversations.NewGetConversationHistoryLogic(r.Context(), svcCtx)
		resp, err := l.GetConversationHistory(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
