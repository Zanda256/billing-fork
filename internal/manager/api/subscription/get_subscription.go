package subscription

import (
	"database/sql"
	"errors"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
)

// GetSubscription retrieves the current user's subscription with enriched data
func GetSubscription(r *request.Request) {
	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Get subscription with enriched data using new Wave 18 service
	subscription, err := r.State.UserSubscriptionService.GetUserSubscription(r.Request.Context(), userCtx.User.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No active subscription found - return null/empty response
			r.SuccessJSONMessage("no active subscription found")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSON(subscription)
}
