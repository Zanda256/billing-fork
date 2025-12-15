package price

import (
	"context"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"

	"github.com/google/uuid"
)

type PriceService struct {
	repo *repo.PriceRepo
}

func NewPriceService(db *db.DB) *PriceService {
	return &PriceService{repo: repo.NewPriceRepo(db)}
}

func (s *PriceService) Create(ctx context.Context, price *models.Price) error {
	return s.repo.Create(ctx, price)
}

func (s *PriceService) GetByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *PriceService) GetByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	return s.repo.GetByProductID(ctx, productID)
}

func (s *PriceService) GetByNMIPlan(ctx context.Context, provider, nmiPlanID string) (*models.Price, error) {
	return s.repo.GetByNMIPlan(ctx, provider, nmiPlanID)
}

func (s *PriceService) GetByCCBillPriceID(ctx context.Context, ccbillPriceID string) (*models.Price, error) {
	return s.repo.GetByCCBillPriceID(ctx, ccbillPriceID)
}

func (s *PriceService) GetActiveByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	return s.repo.GetActiveByProductID(ctx, productID)
}

func (s *PriceService) Update(ctx context.Context, price *models.Price) error {
	return s.repo.Update(ctx, price)
}

func (s *PriceService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
