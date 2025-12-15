package entitlement

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"time"

	"github.com/google/uuid"
)

type EntitlementService struct {
	repo *repo.EntitlementRepo
}

func NewEntitlementService(db *db.DB) *EntitlementService {
	return &EntitlementService{repo: repo.NewEntitlementRepo(db)}
}

// IsEntitled returns true if the user currently has an active entitlement
func (s *EntitlementService) IsEntitled(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	return s.repo.IsEntitled(ctx, userID, entitlement, at)
}

func (s *EntitlementService) HasActiveIndefinite(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	return s.repo.HasActiveIndefinite(ctx, userID, entitlement, at)
}

func (s *EntitlementService) ExistsBySource(ctx context.Context, sourceType models.EntitlementSourceType, sourceID uuid.UUID, entitlement string) (bool, error) {
	return s.repo.ExistsBySource(ctx, sourceType, sourceID, entitlement)
}

func (s *EntitlementService) LatestFiniteWindow(ctx context.Context, userID, entitlement string, at time.Time) (*models.Entitlement, error) {
	return s.repo.GetLatestFiniteActive(ctx, userID, entitlement, at)
}

func (s *EntitlementService) ListByUser(ctx context.Context, userID string) ([]models.Entitlement, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *EntitlementService) ListActiveRecords(ctx context.Context, userID string, at time.Time) ([]models.Entitlement, error) {
	return s.repo.ListActiveRecords(ctx, userID, at)
}

// AppendEntitlementDays appends a finite window of N days right after the latest window's end,
// unless the user currently has an indefinite window (end_at IS NULL). In that case, it is a no-op.
func (s *EntitlementService) AppendEntitlementDays(ctx context.Context, userID, entitlement string, days int, sourceType models.EntitlementSourceType, sourceID *uuid.UUID) (*models.Entitlement, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be > 0")
	}
	now := time.Now()

	exists, err := s.repo.HasActiveIndefinite(ctx, userID, entitlement, now)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("cannot append entitlement while subscription entitlement is active")
	}

	var start time.Time = now
	last, err := s.repo.GetLatestActive(ctx, userID, entitlement)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && last != nil && last.EndAt != nil {
		start = *last.EndAt
	}

	end := start.Add(time.Duration(days) * 24 * time.Hour)
	return s.GrantWindow(ctx, userID, entitlement, start, &end, sourceType, sourceID)
}

// ListActiveEntitlements returns a de-duplicated list of active entitlement names for a user at a point in time.
func (s *EntitlementService) ListActiveEntitlements(ctx context.Context, userID string, at time.Time) ([]string, error) {
	return s.repo.ListActiveEntitlements(ctx, userID, at)
}

// GrantWindow creates a new entitlement window for a user
func (s *EntitlementService) GrantWindow(ctx context.Context, userID, entitlement string, startAt time.Time, endAt *time.Time, sourceType models.EntitlementSourceType, sourceID *uuid.UUID) (*models.Entitlement, error) {
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlement,
		StartAt:     startAt,
		EndAt:       endAt,
		SourceType:  sourceType,
		SourceID:    sourceID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.repo.Insert(ctx, ent); err != nil {
		return nil, err
	}
	return ent, nil
}

// EndActiveBySubscription ends active entitlements for a subscription at a given time
func (s *EntitlementService) EndActiveBySubscription(ctx context.Context, subscriptionID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	return s.repo.EndActiveBySubscription(ctx, subscriptionID, endAt, reason)
}

// EndActiveByPayment ends active entitlements for a one-off payment at a given time
func (s *EntitlementService) EndActiveByPayment(ctx context.Context, paymentID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	return s.repo.EndActiveByPayment(ctx, paymentID, endAt, reason)
}
