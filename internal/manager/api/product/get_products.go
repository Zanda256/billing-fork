package product

import (
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	response2 "github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"net/http"
)

// GetProducts retrieves all available products and prices for subscription
func GetProducts(r *request.Request) {
	products, err := r.State.PublicSubscriptionService.GetAvailableProducts(r.Request.Context())
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	response := response2.NewGetProductsResponse(products)
	r.SuccessJSON(response)
}

// GetSubscribePageData retrieves data needed for the subscription page
func GetSubscribePageData(r *request.Request) {
	data, err := r.State.PublicSubscriptionService.GetSubscriptionPageData(r.Request.Context())
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	response := response2.NewGetSubscribePageDataResponse(data)
	r.SuccessJSON(response)
}
