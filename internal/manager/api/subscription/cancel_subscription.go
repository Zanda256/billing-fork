package subscription

import (
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
)

// CancelSubscription cancels the current user's subscription
func CancelSubscription(r *request.Request) {
	req := new(request.CancelSubscriptionRequest)
	if !r.BindJSON(req) {
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	if err := r.State.UserSubscriptionService.CancelUserSubscription(
		r.Request.Context(),
		userCtx.User.ID,
		req.Feedback,
	); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription cancelled successfully")
}
