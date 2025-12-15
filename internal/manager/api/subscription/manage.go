package subscription

import (
	"context"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
)

func UpdateStatus(r *request.Request) {
	var data UpdateSubscriptionStatusParams
	if !r.BindJSON(&data) {
		return
	}

	// Use new Wave 18 ManageSubscriptionService constructor
	service := NewManageSubscriptionService(
		r.State.SubscriptionService,
		r.State.NotificationQueueService,
	)

	if err := service.UpdateStatus(context.Background(), &data); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription status updated")
}

func ExtendSubscription(r *request.Request) {
	var data ExtendSubscriptionParams
	if !r.BindJSON(&data) {
		return
	}

	// Use new Wave 18 ManageSubscriptionService constructor
	service := NewManageSubscriptionService(
		r.State.SubscriptionService,
		r.State.NotificationQueueService,
	)

	if err := service.ExtendSubscription(context.Background(), &data); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription extended successfully")
}
