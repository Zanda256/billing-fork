package dunning

import "time"

// Dunning policy constants used when attempting to recover failed rebills
// within the same billing cycle for past_due subscriptions.
const (
	// DunningInterval is the fixed delay between dunning attempts.
	DunningInterval = 72 * time.Hour // every 3 days

	// MaxDunningFailures is the maximum number of failed dunning attempts
	// before we stop trying and cancel the subscription.
	MaxDunningFailures = 5
)
