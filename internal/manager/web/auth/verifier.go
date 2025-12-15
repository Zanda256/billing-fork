package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/PaulFidika/authkit/core"
	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/doujins-org/doujins-billing/config"
)

// Claims represents the identity extracted from a verified JWT.
type Claims struct {
	UserID    string
	Email     string
	Username  string
	Roles     []string
	SessionID string
	ExpiresAt time.Time
	Raw       jwt.MapClaims
}

// EmailPtr returns a pointer to the email if present (useful for services expecting *string).
func (c Claims) EmailPtr() *string {
	if c.Email == "" {
		return nil
	}
	e := c.Email
	return &e
}

// Verifier validates bearer tokens against configured issuers/JWKS.
type Verifier interface {
	Verify(token string) (jwt.MapClaims, error)
}

// NewVerifier builds an authkit-backed verifier using billing auth config.
// Supports multiple issuers to accept tokens from both doujins and hentai0.
func NewVerifier(cfg *config.AuthConfig) (Verifier, error) {
	if cfg == nil {
		return nil, errors.New("auth config is required")
	}

	if len(cfg.Issuers) == 0 {
		return nil, errors.New("at least one auth issuer is required")
	}

	// Build IssuerAccept config for each issuer
	issuerAccepts := make([]core.IssuerAccept, 0, len(cfg.Issuers))
	for _, issuer := range cfg.Issuers {
		issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
		if issuer == "" {
			continue
		}

		issCfg := core.IssuerAccept{Issuer: issuer}
		if cfg.ExpectedAudience != "" {
			issCfg.Audience = cfg.ExpectedAudience
		}

		issuerAccepts = append(issuerAccepts, issCfg)
	}

	if len(issuerAccepts) == 0 {
		return nil, errors.New("no valid issuers configured")
	}

	accept := core.AcceptConfig{
		Issuers:    issuerAccepts,
		Algorithms: []string{"RS256"},
		Skew:       60 * time.Second,
	}

	return authgin.NewVerifier(accept), nil
}

// BuildClaimsFromMap converts a raw JWT map into structured claims.
func BuildClaimsFromMap(raw jwt.MapClaims) *Claims {
	if raw == nil {
		return nil
	}
	claims := Claims{Raw: raw}
	claims.UserID = stringVal(raw["sub"])
	claims.Email = stringVal(raw["email"])
	claims.Username = stringVal(raw["preferred_username"])
	if claims.Username == "" {
		claims.Username = stringVal(raw["username"])
	}
	claims.SessionID = stringVal(raw["session_id"])
	claims.Roles = toStringSlice(raw["roles"])

	switch exp := raw["exp"].(type) {
	case float64:
		claims.ExpiresAt = time.Unix(int64(exp), 0)
	case int64:
		claims.ExpiresAt = time.Unix(exp, 0)
	case json.Number:
		if v, err := exp.Int64(); err == nil {
			claims.ExpiresAt = time.Unix(v, 0)
		}
	}

	return &claims
}

func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, el := range t {
			if s, ok := el.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// FormatVerifierError normalises verifier error messages for HTTP responses.
func FormatVerifierError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "missing_token"):
		return "missing_token"
	case strings.Contains(msg, "bad_issuer"):
		return "invalid_issuer"
	case strings.Contains(msg, "bad_audience"):
		return "invalid_audience"
	case strings.Contains(msg, "invalid_token"):
		return "invalid_or_expired_token"
	default:
		return msg
	}
}
