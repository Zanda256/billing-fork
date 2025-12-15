package product

import (
	"context"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"

	"github.com/google/uuid"
)

type ProductService struct {
	repo *repo.ProductRepo
}

func NewProductService(db *db.DB) *ProductService {
	return &ProductService{repo: repo.NewProductRepo(db)}
}

func (s *ProductService) Create(ctx context.Context, product *models.Product) error {
	return s.repo.Create(ctx, product)
}

func (s *ProductService) GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ProductService) GetActive(ctx context.Context) ([]*models.Product, error) {
	return s.repo.GetActive(ctx)
}

func (s *ProductService) Update(ctx context.Context, product *models.Product) error {
	return s.repo.Update(ctx, product)
}

func (s *ProductService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *ProductService) GetBySlug(ctx context.Context, slug string) (*models.Product, error) {
	return s.repo.GetBySlug(ctx, slug)
}
