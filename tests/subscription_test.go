package tests

import (
	"bytes"
	"encoding/json"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestGetProductsEndpoint tests the public products endpoint
func TestGetProductsEndpoint(t *testing.T) {
	server := setupTestServer(t)

	t.Run("GetProducts_Request", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/products", nil)

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Should return some response (OK, error due to missing DB, or rate limited)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusTooManyRequests}, w.Code)
	})
}

// TestGetSubscribePageDataEndpoint tests the subscribe page data endpoint
func TestGetSubscribePageDataEndpoint(t *testing.T) {
	server := setupTestServer(t)

	t.Run("GetSubscribePageData_Request", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/page-data", nil)

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Should return some response (OK, error due to missing DB, or rate limited)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusTooManyRequests}, w.Code)
	})
}

// TestSubscribeEndpoint tests the subscription endpoint with authentication
func TestSubscribeEndpoint(t *testing.T) {
	server, token := setupTestServerWithAuth(t)

	t.Run("Subscribe_RequiresAuth", func(t *testing.T) {
		subscribeData := request.SubscribeRequest{
			SubscribeBodyParams: request.SubscribeBodyParams{
				SubscribeData: subscription.SubscribeData{
					Processor: "nmi", Provider: "mobius",
					PriceID: uuid.New().String(),
				},
			},
		}

		body, _ := json.Marshal(subscribeData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/process/nmi", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		// Should require auth, but may get rate limited first
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusTooManyRequests}, w.Code)
	})

	t.Run("Subscribe_WithAuth", func(t *testing.T) {
		subscribeData := request.SubscribeRequest{
			SubscribeBodyParams: request.SubscribeBodyParams{
				SubscribeData: subscription.SubscribeData{
					Processor: "nmi", Provider: "mobius",
					PriceID: uuid.New().String(),
				},
			},
		}

		body, _ := json.Marshal(subscribeData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/process/nmi", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		server.Handler().ServeHTTP(w, req)

		// Log response for debugging
		logResponse(t, w, "Subscribe_WithAuth")

		// Should not be unauthorized with valid token
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		// May get rate limited (429), internal server error (500), or bad request (400)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
	})

	t.Run("Subscribe_WithRS256Auth", func(t *testing.T) {
		rsServer, rsToken := setupTestServerWithRSAuth(t)
		subscribeData := request.SubscribeRequest{
			SubscribeBodyParams: request.SubscribeBodyParams{
				SubscribeData: subscription.SubscribeData{
					Processor: "nmi", Provider: "mobius",
					PriceID: uuid.New().String(),
				},
			},
		}

		body, _ := json.Marshal(subscribeData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/process/nmi", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+rsToken)

		rsServer.Handler().ServeHTTP(w, req)

		logResponse(t, w, "Subscribe_WithRS256Auth")

		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
	})
}

// TestCancelSubscriptionEndpoint tests the cancel subscription endpoint
func TestCancelSubscriptionEndpoint(t *testing.T) {
	server, token := setupTestServerWithAuth(t)

	t.Run("CancelSubscription_RequiresAuth", func(t *testing.T) {
		cancelData := request.CancelSubscriptionRequest{
			CancelSubscriptionBodyParams: request.CancelSubscriptionBodyParams{
				Feedback: "Too expensive",
			},
		}

		body, _ := json.Marshal(cancelData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		// Should require auth, but may get rate limited first
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusTooManyRequests}, w.Code)
	})

	t.Run("CancelSubscription_WithAuth", func(t *testing.T) {
		cancelData := request.CancelSubscriptionRequest{
			CancelSubscriptionBodyParams: request.CancelSubscriptionBodyParams{
				Feedback: "Too expensive",
			},
		}

		body, _ := json.Marshal(cancelData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		server.Handler().ServeHTTP(w, req)

		// Log response for debugging
		logResponse(t, w, "CancelSubscription_WithAuth")

		// Should not be unauthorized with valid token
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		// May get rate limited (429), internal server error (500), or bad request (400)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
	})
}

// TestGetSubscriptionEndpoints tests various subscription GET endpoints
func TestGetSubscriptionEndpoints(t *testing.T) {
	server, token := setupTestServerWithAuth(t)

	endpoints := []struct {
		name string
		path string
	}{
		{"GetActiveSubscription", "/api/v1/subscriptions/active"},
		{"GetSubscriptionHistory", "/api/v1/subscriptions/history"},
		{"GetUserPayments", "/api/v1/subscriptions/purchases"},
		{"GetMyBillingStatus", "/api/v1/me/status"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+"_RequiresAuth", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", endpoint.path, nil)

			server.Handler().ServeHTTP(w, req)

			// Should require auth, but may get rate limited first
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusTooManyRequests}, w.Code,
				"Endpoint %s should require authentication or be rate limited", endpoint.path)
		})

		t.Run(endpoint.name+"_WithAuth", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", endpoint.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			server.Handler().ServeHTTP(w, req)

			// Should not be unauthorized with valid token
			assert.NotEqual(t, http.StatusUnauthorized, w.Code,
				"Endpoint %s should accept valid auth", endpoint.path)
			// May get rate limited (429), internal server error (500), or bad request (400)
			assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
		})
	}
}

// TestFlexFormURL tests the CCBill FlexForm URL generation
func TestFlexFormURL(t *testing.T) {
	server, token := setupTestServerWithAuth(t)

	t.Run("FlexFormURL_RequiresAuth", func(t *testing.T) {
		requestData := map[string]interface{}{
			"price_id": uuid.New().String(),
		}

		body, _ := json.Marshal(requestData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		// Log response for debugging
		logResponse(t, w, "FlexFormURL_RequiresAuth")

		// Should require auth, but may get rate limited first
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusTooManyRequests}, w.Code)
	})

	t.Run("FlexFormURL_WithAuth", func(t *testing.T) {
		requestData := map[string]interface{}{
			"price_id": uuid.New().String(),
		}

		body, _ := json.Marshal(requestData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		server.Handler().ServeHTTP(w, req)

		// Log response for debugging
		logResponse(t, w, "FlexFormURL_WithAuth")

		// Should not be unauthorized with valid token
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		// May get rate limited (429), internal server error (500), or bad request (400)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
	})
}
