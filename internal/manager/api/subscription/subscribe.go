package subscription

import (
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"

	log "github.com/sirupsen/logrus"
)

func Subscribe(r *request.Request) {
	var req request.SubscribeRequest
	if !r.BindJSON(&req) {
		return
	}

	if req.Processor == "" {
		req.Processor = r.Param("processor")
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	res, err := r.State.SubscriptionService.Subscribe(r.Request.Context(), &req.SubscribeData, userCtx.User)
	if err != nil {
		log.WithError(err).Error("failed to subscribe")
		r.ErrorJSON(500, "Internal server error")
		return
	}

	r.SuccessJSON(res)
}
