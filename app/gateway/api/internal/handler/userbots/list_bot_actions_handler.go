// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package userbots

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/logic/userbots"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListBotActionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := userbots.NewListBotActionsLogic(r.Context(), svcCtx)
		resp, err := l.ListBotActions()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
