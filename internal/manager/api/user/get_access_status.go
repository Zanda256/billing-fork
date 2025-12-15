package user

import (
	"database/sql"
	"errors"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
)

type AccessStatusResponse struct {
	IsPremium bool               `json:"is_premium"`
	Access    []*UserAccessGrant `json:"access"`
}

func GetAccessStatus(r *request.Request) {
	user := r.GetUser()
	if user == nil {
		r.ErrorJSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	if r.State == nil || r.State.UserSubscriptionService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "access service unavailable")
		return
	}

	grants, err := r.State.UserSubscriptionService.GetUserAccessStatus(r.Request.Context(), user.ID)
	switch {
	case err == nil:
		if grants == nil {
			grants = []*UserAccessGrant{}
		}
		resp := AccessStatusResponse{IsPremium: len(grants) > 0, Access: grants}
		r.SuccessJSON(resp)
	case errors.Is(err, sql.ErrNoRows):
		r.SuccessJSON(AccessStatusResponse{IsPremium: false, Access: []*UserAccessGrant{}})
	default:
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
	}
}
