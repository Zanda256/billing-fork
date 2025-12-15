package middleware

import (
	"context"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"github.com/doujins-org/doujins-billing/internal/manager/web/auth"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/pkg/message"
)

// Context keys for downstream consumers.
const (
	UserContextKey   = "user"
	UserIDContextKey = "user_id"
)

// UserContext represents identity details attached to each request.
type UserContext struct {
	User      *user.UserIdentity `json:"user"`
	SessionID string             `json:"session_id"`
	ExpiresAt time.Time          `json:"exp"`
}

// HasRole reports whether the user owns a specific role.
func (u *UserContext) HasRole(role string) bool {
	if u == nil || u.User == nil {
		return false
	}
	for _, r := range u.User.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// AuthRequired enforces the presence of a valid bearer token.
func AuthRequired(verifier auth.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil {
			log.Warn("auth middleware misconfigured: no verifier provided")
			c.JSON(http.StatusInternalServerError, message.Message("authentication disabled"))
			c.Abort()
			return
		}
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			c.JSON(http.StatusUnauthorized, message.Message("authorization header required"))
			c.Abort()
			return
		}
		claims, err := verifier.Verify(token)
		if err != nil {
			log.WithError(err).Warn("jwt verification failed")
			c.JSON(http.StatusUnauthorized, message.Message(auth.FormatVerifierError(err)))
			c.Abort()
			return
		}

		uc := buildUserContext(auth.BuildClaimsFromMap(claims))
		attachUserContext(c, uc)
		c.Next()
	}
}

// OptionalAuth attaches identity if the bearer token is present, otherwise continues.
func OptionalAuth(verifier auth.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			c.Next()
			return
		}
		if verifier == nil {
			log.Warn("auth middleware misconfigured: no verifier provided for optional auth")
			c.Next()
			return
		}
		claims, err := verifier.Verify(token)
		if err != nil {
			log.WithError(err).Debug("optional jwt verification failed")
			c.Next()
			return
		}

		uc := buildUserContext(auth.BuildClaimsFromMap(claims))
		attachUserContext(c, uc)
		c.Next()
	}
}

// AdminRequired ensures the current user has the admin role.
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := GetUserContext(c)
		if ctx == nil || ctx.User == nil {
			c.JSON(http.StatusUnauthorized, message.Message("authentication required"))
			c.Abort()
			return
		}

		if !ctx.HasRole("admin") {
			log.WithField(UserIDContextKey, ctx.User.ID).Warn("admin access denied")
			c.JSON(http.StatusForbidden, message.Message("admin privileges required"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetUserContext retrieves the user context stored in gin.Context.
func GetUserContext(c *gin.Context) *UserContext {
	if val, ok := c.Get(UserContextKey); ok {
		if uc, ok := val.(*UserContext); ok {
			return uc
		}
	}
	return nil
}

// RequireUserContext aborts the request if no authenticated user is present.
func RequireUserContext(c *gin.Context) (*UserContext, bool) {
	uc := GetUserContext(c)
	if uc == nil {
		c.JSON(http.StatusUnauthorized, message.Message("authentication required"))
		return nil, false
	}
	return uc, true
}

func attachUserContext(c *gin.Context, uc *UserContext) {
	if uc == nil {
		return
	}
	c.Set(UserContextKey, uc)
	if uc.User != nil {
		//lint:ignore SA1029 legacy string key usage across handlers
		ctx := context.WithValue(c.Request.Context(), UserIDContextKey, uc.User.ID)
		c.Request = c.Request.WithContext(ctx)
	}
}

func buildUserContext(claims *auth.Claims) *UserContext {
	if claims == nil {
		return nil
	}
	identity := &user.UserIdentity{
		ID:       claims.UserID,
		Username: claims.Username,
		Roles:    claims.Roles,
	}
	if emailPtr := claims.EmailPtr(); emailPtr != nil {
		identity.Email = emailPtr
	}
	return &UserContext{
		User:      identity,
		SessionID: claims.SessionID,
		ExpiresAt: claims.ExpiresAt,
	}
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(header), strings.ToLower(prefix)) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
