package notification

import (
	"context"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type NotificationQueueService struct {
	repo *repo.NotificationQueueRepo
}

type GetNotificationsFilters struct {
	UserID    string `form:"user_id"`
	EventType string `form:"event_type"`
	Seen      *bool  `form:"seen"`
}

func NewNotificationQueueService(db *db.DB) *NotificationQueueService {
	return &NotificationQueueService{repo: repo.NewNotificationQueueRepo(db)}
}

func (s *NotificationQueueService) Create(ctx context.Context, notification *models.NotificationQueue) error {
	return s.repo.Create(ctx, notification)
}

func (s *NotificationQueueService) GetByID(ctx context.Context, id uuid.UUID) (*models.NotificationQueue, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *NotificationQueueService) GetByUserID(ctx context.Context, userID string) ([]*models.NotificationQueue, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *NotificationQueueService) GetUnseenByUserID(ctx context.Context, userID string) ([]*models.NotificationQueue, error) {
	return s.repo.GetUnseenByUserID(ctx, userID)
}

func (s *NotificationQueueService) GetByEventType(ctx context.Context, eventType models.NotificationEventType) ([]*models.NotificationQueue, error) {
	return s.repo.GetByEventType(ctx, eventType)
}

func (s *NotificationQueueService) CountByUserAndEventSince(ctx context.Context, userID string, eventType models.NotificationEventType, since time.Time) (int, error) {
	return s.repo.CountByUserAndEventSince(ctx, userID, eventType, since)
}

func (s *NotificationQueueService) GetUsersWithPendingDigest(ctx context.Context, since time.Time) ([]string, error) {
	return s.repo.GetUsersWithPendingDigest(ctx, since)
}

func (s *NotificationQueueService) GetPendingDigestForUser(ctx context.Context, userID string, since time.Time, limit int) ([]*models.NotificationQueue, error) {
	return s.repo.GetPendingDigestForUser(ctx, userID, since, limit)
}

func (s *NotificationQueueService) MarkAsSeen(ctx context.Context, id uuid.UUID) error {
	return s.repo.MarkAsSeen(ctx, id)
}

func (s *NotificationQueueService) Update(ctx context.Context, notification *models.NotificationQueue) error {
	return s.repo.Update(ctx, notification)
}

func (s *NotificationQueueService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *NotificationQueueService) GetNotifications(ctx context.Context, queryOpts query.QueryOptions[GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
	repoFilters := repo.NotificationFilters{
		UserID:    queryOpts.Filters.UserID,
		EventType: models.NotificationEventType(queryOpts.Filters.EventType),
		Seen:      queryOpts.Filters.Seen,
	}

	repoOpts := query.QueryOptions[repo.NotificationFilters]{
		Filters:  repoFilters,
		Limit:    queryOpts.Limit,
		Offset:   queryOpts.Offset,
		Page:     queryOpts.Page,
		PageSize: queryOpts.PageSize,
		All:      queryOpts.All,
	}

	return s.repo.GetNotifications(ctx, repoOpts)
}
