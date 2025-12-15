package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// OAuthStateManager manages OAuth state/nonce for CSRF protection.
// Reusable across multiple OAuth providers (Google, GitHub, etc).
type OAuthStateManager struct {
	stateMutex    sync.RWMutex
	pendingStates map[string]*StateData
}

// StateData holds temporary state for OAuth flow.
type StateData struct {
	State        string
	Nonce        string
	RedirectPath string
	CreatedAt    time.Time
}

// NewOAuthStateManager creates a new OAuth state manager.
func NewOAuthStateManager() *OAuthStateManager {
	manager := &OAuthStateManager{
		pendingStates: make(map[string]*StateData),
	}

	// Start cleanup goroutine
	go manager.cleanupExpiredStates()

	return manager
}

// CreateState generates a new state/nonce pair and stores it.
func (m *OAuthStateManager) CreateState(redirectPath string) (*StateData, error) {
	state, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	nonce, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	stateData := &StateData{
		State:        state,
		Nonce:        nonce,
		RedirectPath: redirectPath,
		CreatedAt:    time.Now(),
	}

	m.stateMutex.Lock()
	m.pendingStates[state] = stateData
	m.stateMutex.Unlock()

	return stateData, nil
}

// ValidateAndConsumeState validates a state parameter and returns its data.
// The state is removed from the pending map (consumed).
func (m *OAuthStateManager) ValidateAndConsumeState(state string) (*StateData, error) {
	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()

	stateData, exists := m.pendingStates[state]
	if !exists {
		return nil, fmt.Errorf("invalid or expired state parameter")
	}

	// Remove state after validation (prevent reuse)
	delete(m.pendingStates, state)

	return stateData, nil
}

// cleanupExpiredStates periodically removes expired OAuth state data.
func (m *OAuthStateManager) cleanupExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.stateMutex.Lock()
		now := time.Now()
		for state, data := range m.pendingStates {
			// Remove states older than 10 minutes
			if now.Sub(data.CreatedAt) > 10*time.Minute {
				delete(m.pendingStates, state)
			}
		}
		m.stateMutex.Unlock()
	}
}

// generateRandomString generates a cryptographically secure random string of the specified length.
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
