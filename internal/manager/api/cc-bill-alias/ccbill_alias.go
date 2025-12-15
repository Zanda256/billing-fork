package cc_bill_alias

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/data/repo"
	"github.com/doujins-org/doujins-billing/pkg/db"
	"github.com/doujins-org/doujins-billing/pkg/db/models"
	"strings"
)

const (
	ccbillAliasMinLength = 4
	ccbillAliasMaxLength = 16
)

var base32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

type CCBillAliasService struct {
	repo *repo.CCBillUsernameAliasRepo
}

func NewCCBillAliasService(db *db.DB) *CCBillAliasService {
	return &CCBillAliasService{repo: repo.NewCCBillUsernameAliasRepo(db)}
}

func (s *CCBillAliasService) GetOrCreate(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required for alias creation")
	}

	if alias, err := s.repo.GetByUserID(ctx, userID); err == nil {
		return alias.Alias, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		candidate := generateAliasCandidate(userID, attempt)
		model := &models.CCBillUsernameAlias{
			Alias:  candidate,
			UserID: userID,
		}

		if err := s.repo.Create(ctx, model); err != nil {
			if errors.Is(err, repo.ErrCCBillAliasConflict) {
				// Retry with a new candidate or fetch existing record if user already has one
				if alias, aliasErr := s.repo.GetByUserID(ctx, userID); aliasErr == nil {
					return alias.Alias, nil
				}
				continue
			}
			return "", err
		}

		return candidate, nil
	}

	return "", fmt.Errorf("unable to allocate ccbill alias for user %s", userID)
}

func (s *CCBillAliasService) Resolve(ctx context.Context, alias string) (string, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "", fmt.Errorf("ccbill username alias is required")
	}

	model, err := s.repo.GetByAlias(ctx, alias)
	if err != nil {
		return "", err
	}
	return model.UserID, nil
}

func generateAliasCandidate(userID string, attempt int) string {
	base := userID
	if attempt > 0 {
		base = fmt.Sprintf("%s#%d", userID, attempt)
	}
	hash := sha256.Sum256([]byte(base))
	alias := strings.ToLower(base32NoPadding.EncodeToString(hash[:]))
	if len(alias) > ccbillAliasMaxLength {
		alias = alias[:ccbillAliasMaxLength]
	}
	if len(alias) < ccbillAliasMinLength {
		return fmt.Sprintf("user%0*d", ccbillAliasMinLength-4, attempt)
	}
	return alias
}
