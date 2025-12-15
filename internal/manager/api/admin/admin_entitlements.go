package admin

import (
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
	"time"
)

type AdminUserEntitlementsPath struct {
	UserID string `uri:"user_id" binding:"required"`
}

// GetAdminActiveEntitlements returns the list of currently active entitlements for a user
func GetAdminActiveEntitlements(r *request.Request) {
	var path AdminUserEntitlementsPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	svc := r.State.EntitlementService
	if svc == nil {
		r.SuccessJSON([]any{})
		return
	}

	now := time.Now()
	// Active if: revoked_at IS NULL AND start_at <= now AND (end_at IS NULL OR end_at > now)
	ents, err := svc.ListActiveRecords(r.Request.Context(), path.UserID, now)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSON(ents)
}
