package payment

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/internal/manager/api/types"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// ListPaymentMethods returns the user's payment methods (optionally including inactive)
func CreatePaymentMethod(r *request.Request) {
	if r.State == nil || r.State.VaultService == nil {
		r.ErrorJSON(http.StatusServiceUnavailable, "payment vault unavailable")
		return
	}

	user := r.GetUser()
	if user == nil {
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	req := new(request.CreatePaymentMethodRequest)
	if !r.BindJSON(req) {
		return
	}

	if strings.TrimSpace(req.PaymentToken) == "" {
		r.ErrorJSON(http.StatusBadRequest, "payment_token is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()

	createReq := &types.CreateVaultRequest{
		PaymentToken: req.PaymentToken,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Address1:     req.Address1,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
		Country:      req.Country,
		Phone:        req.Phone,
		Company:      req.Company,
		Address2:     req.Address2,
		Provider:     req.Provider,
	}

	if req.Email != "" {
		createReq.Email = req.Email
	} else if user.Email != nil {
		createReq.Email = strings.TrimSpace(*user.Email)
	}

	dbUser := &models.User{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
	}

	pm, err := r.State.VaultService.CreateVault(ctx, dbUser, createReq)
	if err != nil {
		log.WithError(err).WithField("user_id", user.ID).Error("Failed to create payment method")
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	r.SuccessJSON(pm)
}

// UpdatePaymentMethod replaces the stored payment method using a tokenized payload
func UpdatePaymentMethod(r *request.Request) {
	if r.State == nil || r.State.VaultService == nil {
		r.ErrorJSON(http.StatusServiceUnavailable, "payment vault unavailable")
		return
	}

	req := new(request.UpdatePaymentMethodRequest)
	if !r.BindURI(req.Path()) {
		return
	}
	if !r.BindJSON(req.Body()) {
		return
	}

	user := r.GetUser()
	if user == nil {
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	methodID, err := uuid.Parse(req.ID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment method ID format")
		return
	}

	trimmedToken := strings.TrimSpace(req.PaymentToken)
	if trimmedToken == "" {
		r.ErrorJSON(http.StatusBadRequest, "payment_token is required")
		return
	}

	pm, err := r.State.PaymentMethodService.ValidatePaymentMethodOperation(r.Request.Context(), methodID, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPaymentMethodNotFound):
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		case errors.Is(err, ErrPaymentMethodAccessDenied):
			r.ErrorJSON(http.StatusForbidden, "Access denied - you don't own this payment method")
			return
		default:
			log.WithError(err).WithFields(log.Fields{
				"payment_method_id": methodID,
				"user_id":           user.ID,
			}).Error("Failed to validate payment method ownership")
			r.ErrorJSON(http.StatusInternalServerError, "Failed to validate payment method")
			return
		}
	}

	if pm.Processor != models.ProcessorNMI {
		r.ErrorJSON(http.StatusBadRequest, "Only NMI payment methods can be updated")
		return
	}

	updateReq := &types.UpdateVaultRequest{
		PaymentToken: &trimmedToken,
		Provider:     req.Provider,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Address1:     req.Address1,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
		Country:      req.Country,
		Phone:        req.Phone,
		Email:        req.Email,
		Company:      req.Company,
		Address2:     req.Address2,
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()

	updated, err := r.State.VaultService.UpdateVault(ctx, pm, updateReq)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": methodID,
			"user_id":           user.ID,
		}).Error("Failed to update payment method")
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	r.SuccessJSON(updated)
}

func ListPaymentMethods(r *request.Request) {
	req := new(request.ListPaymentMethodsRequest)
	if !r.BindQuery(req) {
		return
	}

	// Fallback if Bind doesn't populate query embedded struct
	if p := r.Request.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			req.Page = v
		} else if err != nil {
			log.WithError(err).WithField("page", p).Error("Invalid page parameter")
			r.ErrorJSON(http.StatusBadRequest, "Invalid page parameter - must be a positive integer")
			return
		}
	}
	if ps := r.Request.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 500 {
			req.PageSize = v
		} else if err != nil {
			log.WithError(err).WithField("page_size", ps).Error("Invalid page_size parameter")
			r.ErrorJSON(http.StatusBadRequest, "Invalid page_size parameter - must be a positive integer")
			return
		} else if v > 500 {
			log.WithField("page_size", v).Error("Page size too large")
			r.ErrorJSON(http.StatusBadRequest, "Page size cannot exceed 500")
			return
		}
	}

	req.SetDefaults()

	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Validate pagination parameters
	if req.Page < 1 {
		r.ErrorJSON(http.StatusBadRequest, "Page must be greater than 0")
		return
	}
	if req.PageSize < 1 || req.PageSize > 500 {
		r.ErrorJSON(http.StatusBadRequest, "Page size must be between 1 and 500")
		return
	}

	methods, totalItems, err := r.State.PaymentMethodService.ListByUserID(
		r.Request.Context(),
		user.ID,
		req.IncludeInactive,
		req.Page,
		req.PageSize,
	)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":          user.ID,
			"include_inactive": req.IncludeInactive,
			"page":             req.Page,
			"page_size":        req.PageSize,
		}).Error("Failed to retrieve payment methods")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to retrieve payment methods")
		return
	}

	totalPages := 0
	if req.PageSize > 0 {
		totalPages = int((totalItems + int64(req.PageSize) - 1) / int64(req.PageSize))
	}

	r.SuccessJSON(response.PaginatedResponse{
		Data:       methods,
		TotalItems: totalItems,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	})
}

// DeletePaymentMethod removes a payment method by ID
func DeletePaymentMethod(r *request.Request) {
	req := new(request.DeletePaymentMethodRequest)
	if !r.BindURI(req.Path()) {
		return
	}

	// Validate UUID format
	id, err := uuid.Parse(req.ID)
	if err != nil {
		log.WithError(err).WithField("id", req.ID).Error("Invalid payment method ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment method ID format")
		return
	}

	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Validate ownership and get payment method details
	paymentMethod, err := r.State.PaymentMethodService.ValidatePaymentMethodOperation(r.Request.Context(), id, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPaymentMethodNotFound):
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found for deletion")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		case errors.Is(err, ErrPaymentMethodAccessDenied):
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Unauthorized payment method deletion attempt")
			r.ErrorJSON(http.StatusForbidden, "Access denied - you don't own this payment method")
			return
		default:
			log.WithError(err).WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Error("Failed to validate payment method ownership")
			r.ErrorJSON(http.StatusInternalServerError, "Failed to validate payment method")
			return
		}
	}

	// Perform the deletion
	if err := r.State.PaymentMethodService.Delete(r.Request.Context(), id); err != nil {
		if errors.Is(err, ErrPaymentMethodNotFound) {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found during deletion")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to delete payment method")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to delete payment method")
		return
	}

	// Log successful deletion for audit purposes
	log.WithFields(log.Fields{
		"payment_method_id": id,
		"user_id":           user.ID,
		"processor":         paymentMethod.Processor,
	}).Info("Payment method successfully deleted")

	r.SuccessJSON(map[string]any{
		"success": true,
		"message": "Payment method deleted successfully",
	})
}

// ActivatePaymentMethod activates a given payment method ID
func ActivatePaymentMethod(r *request.Request) {
	req := new(request.ActivatePaymentMethodRequest)
	if !r.BindURI(req.Path()) {
		return
	}

	// Validate UUID format
	id, err := uuid.Parse(req.ID)
	if err != nil {
		log.WithError(err).WithField("id", req.ID).Error("Invalid payment method ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment method ID format")
		return
	}

	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Validate ownership and get payment method details
	paymentMethod, err := r.State.PaymentMethodService.ValidatePaymentMethodOperation(r.Request.Context(), id, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPaymentMethodNotFound):
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found for activation")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		case errors.Is(err, ErrPaymentMethodAccessDenied):
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Unauthorized payment method activation attempt")
			r.ErrorJSON(http.StatusForbidden, "Access denied - you don't own this payment method")
			return
		default:
			log.WithError(err).WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Error("Failed to validate payment method ownership")
			r.ErrorJSON(http.StatusInternalServerError, "Failed to validate payment method")
			return
		}
	}

	// Check if payment method is already active
	if paymentMethod.IsActive {
		log.WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Info("Payment method is already active")
		r.SuccessJSON(map[string]any{
			"success": true,
			"message": "Payment method is already active",
		})
		return
	}

	// Check if payment method has failure reasons that prevent activation
	if paymentMethod.FailureReason != nil && *paymentMethod.FailureReason != "" {
		log.WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
			"failure_reason":    *paymentMethod.FailureReason,
		}).Warn("Cannot activate payment method with failure reason")
		r.ErrorJSON(http.StatusConflict, "Cannot activate payment method: "+*paymentMethod.FailureReason)
		return
	}

	// Perform the activation
	if err := r.State.PaymentMethodService.ActivateByID(r.Request.Context(), id); err != nil {
		if errors.Is(err, ErrPaymentMethodNotFound) {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found during activation")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to activate payment method")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to activate payment method")
		return
	}

	// Log successful activation for audit purposes
	log.WithFields(log.Fields{
		"payment_method_id": id,
		"user_id":           user.ID,
		"processor":         paymentMethod.Processor,
	}).Info("Payment method successfully activated")

	r.SuccessJSON(map[string]any{
		"success": true,
		"message": "Payment method activated successfully",
	})
}
