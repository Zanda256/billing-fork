package subscription

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/api/price"
	"github.com/doujins-org/doujins-billing/internal/manager/api/product"
	"github.com/doujins-org/doujins-billing/internal/manager/api/types"
	"github.com/doujins-org/doujins-billing/pkg/db/models"

	log "github.com/sirupsen/logrus"
)

// PublicSubscriptionService handles public-facing subscription operations
type PublicSubscriptionService struct {
	ProductService *product.ProductService
	PriceService   *price.PriceService
}

// GetAvailableProducts returns all active products with their prices for public consumption
func (s *PublicSubscriptionService) GetAvailableProducts(ctx context.Context) ([]*types.PublicProductResponse, error) {
	products, err := s.ProductService.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get products: %w", err)
	}

	responses := make([]*types.PublicProductResponse, len(products))
	for i, product := range products {
		responses[i] = &types.PublicProductResponse{
			Product: product,
		}

		// Get prices for this product
		prices, err := s.PriceService.GetByProductID(ctx, product.ID)
		if err != nil {
			log.WithFields(log.Fields{
				"product_id": product.ID,
				"error":      err.Error(),
			}).Warn("Failed to load prices for product in public API")
			prices = []*models.Price{}
		}

		responses[i].Prices = prices
	}

	return responses, nil
}

// GetSubscriptionPageData returns data needed for the subscription page
func (s *PublicSubscriptionService) GetSubscriptionPageData(ctx context.Context) (map[string]any, error) {
	products, err := s.GetAvailableProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get products: %w", err)
	}

	// Build subscription page data
	data := map[string]any{
		"products": products,
		"features": map[string]any{
			"premium": []string{
				"Access to premium content",
				"HD quality downloads",
				"Priority support",
				"No advertisements",
			},
		},
		"currency": "USD", // Currently USD only, future enhancement will support multiple currencies
	}

	return data, nil
}

// NewPublicSubscriptionService creates a new PublicSubscriptionService
func NewPublicSubscriptionService(
	productService *product.ProductService,
	priceService *price.PriceService,
) *PublicSubscriptionService {
	return &PublicSubscriptionService{
		ProductService: productService,
		PriceService:   priceService,
	}
}
