package notification

import (
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"github.com/doujins-org/doujins-billing/internal/manager/web/response"
	"net/http"
	"strconv"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

// GetNotifications returns paginated in-app notifications for the current user
func GetNotifications(r *request.Request) {
	user := r.GetUser()

	// Parse query params
	page, _ := strconv.Atoi(r.Request.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.Request.URL.Query().Get("page_size"))
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	seenParam := r.Request.URL.Query().Get("seen")
	var seen *bool
	if seenParam == "true" {
		t := true
		seen = &t
	}
	if seenParam == "false" {
		f := false
		seen = &f
	}

	q := &query.QueryOptions[GetNotificationsFilters]{
		Page:     page,
		PageSize: pageSize,
		Filters:  GetNotificationsFilters{UserID: user.ID, Seen: seen},
	}

	items, _, err := r.State.UserSubscriptionService.GetUserNotifications(r.Request.Context(), user.ID, q)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSON(response.PaginatedResponse{Data: items, TotalItems: q.TotalItems, Page: page, PageSize: pageSize})
}

// MarkNotificationRead marks a notification as read for the user
func MarkNotificationRead(r *request.Request) {
	user := r.GetUser()
	idStr := r.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid notification ID")
		return
	}
	if err := r.State.UserSubscriptionService.MarkNotificationRead(r.Request.Context(), user.ID, id); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSONMessage("notification marked as read")
}

// GetUnreadNotificationCount returns the user's unread count
func GetUnreadNotificationCount(r *request.Request) {
	user := r.GetUser()
	f := false
	q := &query.QueryOptions[GetNotificationsFilters]{
		Page: 1, PageSize: 1,
		Filters: GetNotificationsFilters{UserID: user.ID, Seen: &f},
	}
	// We only need total count; items ignored
	if _, _, err := r.State.UserSubscriptionService.GetUserNotifications(r.Request.Context(), user.ID, q); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(map[string]any{"unread_count": q.TotalItems})
}
