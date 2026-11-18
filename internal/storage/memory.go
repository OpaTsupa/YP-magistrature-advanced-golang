// Package storage stores shortened links in process memory.
package storage

import "sync"

type UserURL struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

type Memory struct {
	mu           sync.RWMutex
	originalByID map[string]string
	urlsByUser   map[string][]UserURL
}

func NewMemory() *Memory {
	return &Memory{
		originalByID: make(map[string]string),
		urlsByUser:   make(map[string][]UserURL),
	}
}

// Save returns false when the short identifier already exists.
// Both indexes are updated under one lock.
func (m *Memory) Save(id, userID, originalURL, shortURL string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.originalByID[id]; exists {
		return false
	}
	m.originalByID[id] = originalURL
	m.urlsByUser[userID] = append(m.urlsByUser[userID], UserURL{
		ShortURL: shortURL, OriginalURL: originalURL,
	})
	return true
}

func (m *Memory) Resolve(id string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	url, ok := m.originalByID[id]
	return url, ok
}

// List returns a copy to keep internal slices isolated from the caller.
func (m *Memory) List(userID string) []UserURL {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := m.urlsByUser[userID]
	result := make([]UserURL, len(items))
	copy(result, items)
	return result
}
