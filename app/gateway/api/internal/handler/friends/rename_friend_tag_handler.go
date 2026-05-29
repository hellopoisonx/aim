// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/logic/friends"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func RenameFriendTagHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RenameFriendTagRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := friends.NewRenameFriendTagLogic(r.Context(), svcCtx)
		resp, err := l.RenameFriendTag(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
