package idempotency

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"

	"github.com/google/uuid"
)

type IdempotencyService struct {
	repo *repo.IdempotencyRepo
}

func NewIdempotencyService(db *db.DB) *IdempotencyService {
	return &IdempotencyService{repo: repo.NewIdempotencyRepo(db)}
}

func (g *IdempotencyService) Begin(ctx context.Context, operation, key string, userID *string) (*models.IdempotencyRequest, bool, error) {
	operation = strings.TrimSpace(operation)
	key = strings.TrimSpace(key)
	if operation == "" || key == "" {
		return nil, false, errors.New("idempotency operation and key are required")
	}
	req, err := g.repo.GetByOperationAndKey(ctx, operation, key)
	if err == nil {
		return req, true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	req = &models.IdempotencyRequest{
		Operation: operation,
		Key:       key,
		Status:    "pending",
		UserID:    userID,
	}
	if err := g.repo.Create(ctx, req); err != nil {
		if errors.Is(err, repo.ErrIdempotencyConflict) {
			existing, getErr := g.repo.GetByOperationAndKey(ctx, operation, key)
			if getErr != nil {
				return nil, false, getErr
			}
			return existing, true, nil
		}
		return nil, false, err
	}
	return req, false, nil
}

func (g *IdempotencyService) Complete(ctx context.Context, id uuid.UUID, result json.RawMessage) error {
	return g.repo.UpdateStatus(ctx, id, "success", result)
}

func (g *IdempotencyService) Fail(ctx context.Context, id uuid.UUID, failure error) error {
	var payload []byte
	if failure != nil {
		payload, _ = json.Marshal(map[string]string{"error": failure.Error()})
	}
	return g.repo.UpdateStatus(ctx, id, "failed", payload)
}

// GenerateForChargeSuccess generates key for successful charge
// Format: v5({processor}:{transaction_id})
func (g *IdempotencyService) GenerateForChargeSuccess(processor, transactionID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // UUID v5 namespace
	data := fmt.Sprintf("%s:%s", processor, transactionID)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForRenewal generates key for subscription renewal
// Format: v5({subscription_id}:{period_end_iso})
func (g *IdempotencyService) GenerateForRenewal(subscriptionID uuid.UUID, periodEndISO string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:%s", subscriptionID.String(), periodEndISO)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForCancelNow generates key for immediate cancellation
// Format: v5({subscription_id}:cancel:{event_id})
func (g *IdempotencyService) GenerateForCancelNow(subscriptionID uuid.UUID, eventID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b812-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:cancel:%s", subscriptionID.String(), eventID)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForTrialStart generates key for trial start
// Format: v5({subscription_id}:trial:{trial_end_iso})
func (g *IdempotencyService) GenerateForTrialStart(subscriptionID uuid.UUID, trialEndISO string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b813-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:trial:%s", subscriptionID.String(), trialEndISO)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForGracePeriod generates key for grace period
// Format: v5({subscription_id}:grace:{grace_until_iso})
func (g *IdempotencyService) GenerateForGracePeriod(subscriptionID uuid.UUID, graceUntilISO string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b814-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:grace:%s", subscriptionID.String(), graceUntilISO)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForOneOff generates key for one-off/gift purchases
// Format: v5({user_id}:oneoff:{product_id}:{timestamp})
func (g *IdempotencyService) GenerateForOneOff(userID, productID uuid.UUID, timestamp string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b815-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:oneoff:%s:%s", userID.String(), productID.String(), timestamp)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForRefund generates key for refund operations
// Format: v5({payment_id}:refund:{timestamp})
func (g *IdempotencyService) GenerateForRefund(paymentID uuid.UUID, timestamp string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b816-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:refund:%s", paymentID.String(), timestamp)
	return uuid.NewSHA1(namespace, []byte(data))
}
