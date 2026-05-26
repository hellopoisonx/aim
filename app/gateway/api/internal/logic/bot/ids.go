package bot

import (
	"strconv"
	"strings"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

func formatID(id int64) string {
	if id == 0 {
		return "0"
	}
	return strconv.FormatInt(id, 10)
}

func parseRequiredID(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errorx.NewCodeError(errorx.CodeBadInput, field+" is required")
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errorx.NewCodeError(errorx.CodeBadInput, field+" must be a positive decimal string")
	}

	return id, nil
}
