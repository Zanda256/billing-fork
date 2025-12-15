package server

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/admin"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerAdminRoutes() {
	api := s.adminHandler.Group("/v1")
	api.PUT("/subscriptions/:id/extend", s.wrap(subscription.ExtendSubscription))
	api.POST("/subscriptions/:id/cancel", s.wrap(subscription.CancelSubscription))
	api.GET("/subscriptions/:id/details", s.wrap(subscription.GetSubscription))
	api.GET("/subscriptions/dashboard-metrics", s.wrap(admin.GetAdminDashboardMetrics))
	api.GET("/subscriptions/daily-metrics", s.wrap(admin.GetAdminDailyMetrics))
	api.GET("/subscriptions/processor-metrics", s.wrap(admin.GetAdminProcessorMetrics))
	api.GET("/users/:user_id/entitlements", s.wrap(admin.GetAdminActiveEntitlements))

	s.adminHandler.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-admin"})
	})
	s.adminHandler.HEAD("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
