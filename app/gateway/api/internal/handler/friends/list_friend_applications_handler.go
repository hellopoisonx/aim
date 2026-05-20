// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/logic/friends"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListFriendApplicationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := friends.NewListFriendApplicationsLogic(r.Context(), svcCtx)
		resp, err := l.ListFriendApplications()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
