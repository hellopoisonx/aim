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

func CompleteAttachmentUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CompleteAttachmentUploadRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := attachments.NewCompleteAttachmentUploadLogic(r.Context(), svcCtx)
		resp, err := l.CompleteAttachmentUpload(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
