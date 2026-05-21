// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package presence

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/logic/presence"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetFriendsPresenceHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := presence.NewGetFriendsPresenceLogic(r.Context(), svcCtx)
		resp, err := l.GetFriendsPresence()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
