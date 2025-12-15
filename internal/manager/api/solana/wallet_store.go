package solana

import (
	"sync"
)

// SolanaWalletStore provides an in-memory per-process store of user -> wallets.
// This is a scaffold to support wallet endpoints without introducing schema changes.
type SolanaWalletStore struct {
	mu      sync.RWMutex
	wallets map[string]map[string]struct{}
}

func NewSolanaWalletStore() *SolanaWalletStore {
	return &SolanaWalletStore{wallets: make(map[string]map[string]struct{})}
}

func (s *SolanaWalletStore) List(userID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.wallets[userID]
	res := make([]string, 0, len(m))
	for w := range m {
		res = append(res, w)
	}
	return res
}

func (s *SolanaWalletStore) Add(userID string, wallet string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wallets[userID] == nil {
		s.wallets[userID] = make(map[string]struct{})
	}
	s.wallets[userID][wallet] = struct{}{}
}

func (s *SolanaWalletStore) Remove(userID string, wallet string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.wallets[userID]; m != nil {
		delete(m, wallet)
		if len(m) == 0 {
			delete(s.wallets, userID)
		}
	}
}

func (s *SolanaWalletStore) Clear(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.wallets, userID)
}
