package cc_bill_alias

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateAliasCandidate(t *testing.T) {
	alias := generateAliasCandidate("user-123e4567-e89b-12d3-a456-426614174000", 0)
	require.Len(t, alias, ccbillAliasMaxLength)
	require.True(t, regexp.MustCompile(`^[a-z0-9]+$`).MatchString(alias))

	aliasRetry := generateAliasCandidate("user-123e4567-e89b-12d3-a456-426614174000", 1)
	require.NotEqual(t, alias, aliasRetry)
}
