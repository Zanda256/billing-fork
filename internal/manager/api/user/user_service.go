package user

import (
	"github.com/doujins-org/doujins-billing/pkg/db"
)

// UserService provides minimal user lookups for legacy call sites.
// In this project, the external IdP is the source of truth;
// most flows should carry the subject (`user.ID`) directly rather than lookups.
type UserService struct {
	db *db.DB
}

func NewUserService(dbx *db.DB) *UserService { return &UserService{db: dbx} }
