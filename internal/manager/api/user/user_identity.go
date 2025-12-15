package user

// UserIdentity represents the minimal user information needed by billing
type UserIdentity struct {
	ID       string
	Email    *string
	Username string
	Roles    []string
}

// No user directory lookups: the IdP is the source of truth via JWT claims.
