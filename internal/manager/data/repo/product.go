package repo

import (
	"context"
	"errors"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"

	"github.com/google/uuid"
)

type ProductRepo struct {
	db *db.DB
}

func NewProductRepo(d *db.DB) *ProductRepo { return &ProductRepo{db: d} }

func (r *ProductRepo) Create(ctx context.Context, product *models.Product) error {
	res, err := r.db.GetDB().NewInsert().Model(product).Exec(ctx)
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

func (r *ProductRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	product := new(models.Product)
	if err := r.db.GetDB().NewSelect().Model(product).Where("prod.id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return product, nil
}

func (r *ProductRepo) GetActive(ctx context.Context) ([]*models.Product, error) {
	products := []*models.Product{}
	if err := r.db.GetDB().NewSelect().Model(&products).Where("prod.is_active = ?", true).Scan(ctx); err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductRepo) Update(ctx context.Context, product *models.Product) error {
	res, err := r.db.GetDB().NewUpdate().Model(product).WherePK().Exec(ctx)
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

func (r *ProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models.Product)(nil)).Where("prod.id = ?", id).Exec(ctx)
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

func (r *ProductRepo) GetBySlug(ctx context.Context, slug string) (*models.Product, error) {
	product := new(models.Product)
	if err := r.db.GetDB().NewSelect().Model(product).Where("prod.slug = ?", slug).Scan(ctx); err != nil {
		return nil, err
	}
	return product, nil
}
