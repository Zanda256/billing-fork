package user

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"net/http"
	"strconv"

	"github.com/doujins-org/doujins-billing/pkg/query"
)

// GetUserPayments retrieves the user's one-off payments
func GetUserPayments(r *request.Request) {
	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Parse query parameters
	limit, _ := strconv.Atoi(r.Request.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	offset, _ := strconv.Atoi(r.Request.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	paymentType := r.Request.URL.Query().Get("type")

	// Build query options
	queryOpts := &query.QueryOptions[payment.GetPaymentsFilters]{
		Limit:   limit,
		Offset:  offset,
		Filters: payment.GetPaymentsFilters{},
	}

	if paymentType != "" {
		queryOpts.Filters.Processor = paymentType
	}

	payments, _, err := r.State.UserSubscriptionService.GetUserPayments(
		r.Request.Context(),
		userCtx.User.ID,
		queryOpts,
	)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	response := response.PaginatedResponse{Data: payments, TotalItems: queryOpts.TotalItems}
	r.SuccessJSON(response)
}
