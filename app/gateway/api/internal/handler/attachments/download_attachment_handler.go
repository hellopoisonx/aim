// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package attachments

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/logic/attachments"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func DownloadAttachmentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DownloadAttachmentRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := attachments.NewDownloadAttachmentLogic(r.Context(), svcCtx)
		resp, err := l.DownloadAttachment(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
