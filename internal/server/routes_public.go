package server

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/billing"
	"github.com/doujins-org/doujins-billing/internal/manager/api/notification"
	"github.com/doujins-org/doujins-billing/internal/manager/api/payment"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/solana"
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/webhook"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerPublicRoutes() {
	// Root: simple JSON banner for API servers
	s.publicHandler.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":   "billing",
			"status":    "ok",
			"endpoints": []string{"/health/live", "/health/ready", "/v1"},
		})
	})

	api := s.publicHandler.Group("/v1")

	subscriptions := api.Group("/subscriptions")
	subscriptions.GET("/products", s.wrap(product.GetProducts))
	subscriptions.GET("/page-data", s.wrap(product.GetSubscribePageData))

	subscriptions.Use(middleware.AuthRequired(s.authVerifier))
	subscriptions.POST("/process/:processor", s.wrap(subscription.Subscribe))
	subscriptions.POST("/ccbill/flexform-url", s.wrap(billing.GenerateFlexFormURL))
	subscriptions.POST("/cancel", s.wrap(subscription.CancelSubscription))
	subscriptions.GET("/active", s.wrap(subscription.GetSubscription))
	subscriptions.GET("/history", s.wrap(subscription.GetSubscriptionHistory))
	subscriptions.GET("/purchases", s.wrap(user.GetUserPayments))

	webhooks := api.Group("/subscriptions/webhook")
	webhooks.POST("/:processor", s.wrap(webhook.Webhook))
	webhooks.POST("/:processor/:provider", s.wrap(webhook.Webhook))

	pms := api.Group("/payment-methods")
	pms.Use(middleware.AuthRequired(s.authVerifier))
	pms.POST("", s.wrap(payment.CreatePaymentMethod))
	pms.GET("", s.wrap(payment.ListPaymentMethods))
	pms.PUT(":id", s.wrap(payment.UpdatePaymentMethod))
	pms.DELETE(":id", s.wrap(payment.DeletePaymentMethod))
	pms.PUT(":id/activate", s.wrap(payment.ActivatePaymentMethod))

	notifications := api.Group("/notifications")
	notifications.Use(middleware.AuthRequired(s.authVerifier))
	notifications.GET("", s.wrap(notification.GetNotifications))
	notifications.GET("/unread-count", s.wrap(notification.GetUnreadNotificationCount))
	notifications.POST(":id/read", s.wrap(notification.MarkNotificationRead))

	wallet := api.Group("/wallet/solana")
	wallet.Use(middleware.AuthRequired(s.authVerifier))
	wallet.GET("", s.wrap(solana.ListSolanaWallets))
	wallet.GET("/linked", s.wrap(solana.GetSolanaWallet))
	wallet.POST("/challenge", s.wrap(solana.GenerateSolanaWalletChallenge))
	wallet.POST("/verify", s.wrap(solana.VerifySolanaWallet))
	wallet.DELETE("", s.wrap(solana.DeleteSolanaWallet))

	solana := api.Group("/solana")
	solana.GET("/tokens", s.wrap(solana.GetSupportedTokens))
	solana.Use(middleware.AuthRequired(s.authVerifier))
	solana.POST("/generate", s.wrap(solana.GeneratePayment))
	solana.POST("/submit", s.wrap(solana.SubmitPayment))
	solana.POST("/qr", s.wrap(solana.GenerateSolanaPayQR))
	solana.GET("/check", s.wrap(solana.CheckSolanaPayment))

	access := api.Group("/access")
	access.Use(middleware.AuthRequired(s.authVerifier))
	access.GET("", s.wrap(user.GetAccessStatus))

	api.GET("/me/status", s.wrap(billing.GetMyBillingStatus))

	s.publicHandler.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})

	s.publicHandler.GET("/health/ready", s.readyHandler)

	// Kubernetes-style health check endpoints (aliases)
	s.publicHandler.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})
	s.publicHandler.GET("/readyz", s.readyHandler)
}

func (s *Server) readyHandler(c *gin.Context) {
	ctx := c.Request.Context()
	checks := gin.H{
		"db":      "ok",
		"redis":   "ok",
		"authkit": "ok",
	}

	// Check database (critical)
	var one int
	if s.runtime != nil && s.runtime.DB != nil {
		if err := s.runtime.DB.GetDB().NewSelect().ColumnExpr("1").Scan(ctx, &one); err != nil {
			checks["db"] = "down"
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
			return
		}
	} else {
		checks["db"] = "missing"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
		return
	}

	// Check Redis (critical for billing operations)
	if s.runtime != nil && s.runtime.RedisClient != nil {
		if _, err := s.runtime.RedisClient.Ping(ctx).Result(); err != nil {
			checks["redis"] = "down"
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
			return
		}
	} else {
		checks["redis"] = "missing"
	}

	// Check AuthKit verifier (critical for authentication)
	if s.authVerifier == nil {
		checks["authkit"] = "not_initialized"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready", "checks": checks})
}
