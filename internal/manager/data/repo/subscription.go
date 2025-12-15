package repo

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/pkg/db"
	models2 "github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"

	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SubscriptionFilters struct {
	UserID    string
	Status    string
	PriceID   uuid.UUID
	Processor string
}

type SubscriptionRepo struct {
	db *db.DB
}

func NewSubscriptionRepo(d *db.DB) *SubscriptionRepo { return &SubscriptionRepo{db: d} }

func (r *SubscriptionRepo) Create(ctx context.Context, s *models2.Subscription) error {
	res, err := r.db.GetDB().NewInsert().Model(s).Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *SubscriptionRepo) Update(ctx context.Context, s *models2.Subscription) error {
	res, err := r.db.GetDB().NewUpdate().Model(s).WherePK().Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *SubscriptionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models2.Subscription)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *SubscriptionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models2.Subscription, error) {
	sub := new(models2.Subscription)
	err := r.selectWithDetails(sub).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetLatestByUserID(ctx context.Context, userID string) (*models2.Subscription, error) {
	sub := new(models2.Subscription)
	err := r.selectWithDetails(sub).
		Where("sub.user_id = ?", userID).
		Order("sub.created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetByUserIDAndPriceID(ctx context.Context, userID string, priceID uuid.UUID) (*models2.Subscription, error) {
	sub := new(models2.Subscription)
	err := r.selectWithDetails(sub).
		Where("sub.user_id = ?", userID).
		Where("sub.price_id = ?", priceID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetActiveSubscription(ctx context.Context, userID string) (*models2.Subscription, error) {
	sub := new(models2.Subscription)
	err := r.selectWithDetails(sub).
		Where("sub.user_id = ?", userID).
		Where("sub.status = ?", models2.StatusActive).
		Where("(sub.current_period_ends_at IS NULL OR sub.current_period_ends_at > NOW())").
		Order("sub.created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetByProcessorSubscriptionID(ctx context.Context, processor, provider, processorSubscriptionID string) (*models2.Subscription, error) {
	sub := new(models2.Subscription)
	query := r.selectWithDetails(sub).
		Where("sub.processor = ?", processor).
		Where("sub.processor_subscription_id = ?", processorSubscriptionID)

	if strings.EqualFold(processor, string(models2.ProcessorNMI)) {
		provider = strings.TrimSpace(strings.ToLower(provider))
		if provider == "" {
			provider = "mobius"
		}

		if provider == "mobius" {
			query = query.Where("(sub.processor_provider = ? OR sub.processor_provider IS NULL OR sub.processor_provider = '')", provider)
		} else {
			query = query.Where("sub.processor_provider = ?", provider)
		}
	}

	err := query.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetActiveSubscriptionsByUserID(ctx context.Context, userID string) ([]models2.Subscription, error) {
	subs := []models2.Subscription{}
	query := r.selectWithDetails(&subs).
		Where("sub.user_id = ?", userID).
		Where("sub.status = ?", models2.StatusActive).
		Order("sub.created_at DESC")

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *SubscriptionRepo) GetSubscriptionsByProcessorAndUserID(ctx context.Context, userID string, processor models2.Processor) ([]models2.Subscription, error) {
	subs := []models2.Subscription{}
	query := r.selectWithDetails(&subs).
		Where("sub.user_id = ?", userID).
		Where("sub.processor = ?", processor).
		Order("sub.created_at DESC")

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *SubscriptionRepo) GetActiveSubscriptionsByProcessor(ctx context.Context, processor string) ([]*models2.Subscription, error) {
	subs := []*models2.Subscription{}
	query := r.selectWithDetails(&subs).
		Where("sub.processor = ?", processor).
		Where("sub.status = ?", models2.StatusActive)

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *SubscriptionRepo) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]models2.Subscription, int, error) {
	offset := (page - 1) * pageSize
	countQuery := r.db.GetDB().NewSelect().Model((*models2.Subscription)(nil)).Where("sub.user_id = ?", userID)
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	subs := []models2.Subscription{}
	dataQuery := r.selectWithDetails(&subs).
		Where("sub.user_id = ?", userID).
		Order("sub.created_at DESC").
		Limit(pageSize).
		Offset(offset)

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

func (r *SubscriptionRepo) GetSubscriptionsWithDetailsForUser(ctx context.Context, userID string, page, pageSize int) ([]models2.Subscription, int, error) {
	offset := (page - 1) * pageSize
	countQuery := r.db.GetDB().NewSelect().Model((*models2.Subscription)(nil)).Where("sub.user_id = ?", userID)
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	subs := []models2.Subscription{}
	dataQuery := r.selectWithDetails(&subs).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset)

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

func (r *SubscriptionRepo) GetSubscribers(ctx context.Context, params query.QueryOptions[SubscriptionFilters]) ([]*models2.Subscription, int64, error) {
	base := r.db.GetDB().NewSelect().Model((*models2.Subscription)(nil))
	base = applySubscriptionFilters(base, params.Filters)

	total, err := base.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	subs := []*models2.Subscription{}
	dataQuery := r.db.GetDB().NewSelect().Model(&subs)
	dataQuery = applySubscriptionFilters(dataQuery, params.Filters)

	if params.Limit > 0 {
		dataQuery = dataQuery.Limit(params.Limit)
	}
	if params.Offset > 0 {
		dataQuery = dataQuery.Offset(params.Offset)
	}

	dataQuery = dataQuery.Order("created_at DESC")

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subs, int64(total), nil
}

func applySubscriptionFilters(q *bun.SelectQuery, filters SubscriptionFilters) *bun.SelectQuery {
	if filters.UserID != "" {
		q = q.Where("sub.user_id = ?", filters.UserID)
	}
	if filters.Status != "" {
		q = q.Where("sub.status = ?", filters.Status)
	}
	if filters.PriceID != uuid.Nil {
		q = q.Where("sub.price_id = ?", filters.PriceID)
	}
	if filters.Processor != "" {
		q = q.Where("sub.processor = ?", filters.Processor)
	}
	return q
}

func (r *SubscriptionRepo) selectWithDetails(model any) *bun.SelectQuery {
	return r.db.GetDB().NewSelect().Model(model).
		Relation("Price").
		Relation("PaymentMethod")
}
