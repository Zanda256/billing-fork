package admin

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/billing"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"
	"time"
)

// GET /api/v1/subscriptions/dashboard-metrics
func GetAdminDashboardMetrics(r *request.Request) {
	svc := billing.NewBillingAnalyticsService(r.State.DB)
	metrics, err := svc.GetDashboardMetrics(r.Request.Context())
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(metrics)
}

// GET /api/v1/subscriptions/daily-metrics?start=YYYY-MM-DD&end=YYYY-MM-DD
func GetAdminDailyMetrics(r *request.Request) {
	startStr := r.Query("start")
	endStr := r.Query("end")
	if startStr == "" || endStr == "" {
		r.ErrorJSON(http.StatusBadRequest, "start and end query params are required (YYYY-MM-DD)")
		return
	}
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid start date")
		return
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid end date")
		return
	}

	svc := billing.NewBillingAnalyticsService(r.State.DB)
	metrics, err := svc.GetDailyMetrics(r.Request.Context(), start, end)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(metrics)
}

// GET /api/v1/subscriptions/processor-metrics
func GetAdminProcessorMetrics(r *request.Request) {
	svc := billing.NewBillingAnalyticsService(r.State.DB)
	metrics, err := svc.GetMetricsByProcessor(r.Request.Context())
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(metrics)
}
