package runner

import (
	"encoding/base64"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

type StorageItem struct {
	Value     string `json:"v"` // Base64 encoded JSON value
	ExpiresAt int64  `json:"e"` // Expiration timestamp in milliseconds, 0 = never expire
}

type Storage struct {
	mu   sync.RWMutex
	data map[string]*StorageItem
}

func NewStorage() *Storage {
	return &Storage{
		data: make(map[string]*StorageItem),
	}
}

func (s *Storage) Get(key string) (value []byte, expiresAt int64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.data[key]
	if !exists {
		return nil, 0, false
	}

	if item.ExpiresAt > 0 && item.ExpiresAt < time.Now().UnixMilli() {
		delete(s.data, key)
		return nil, 0, false
	}

	decoded, err := base64.StdEncoding.DecodeString(item.Value)
	if err != nil {
		return nil, 0, false
	}

	return decoded, item.ExpiresAt, true
}

func (s *Storage) Set(key string, value []byte, ttlMs ...int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiresAt int64
	if len(ttlMs) > 0 && ttlMs[0] > 0 {
		expiresAt = time.Now().UnixMilli() + ttlMs[0]
	}

	encoded := base64.StdEncoding.EncodeToString(value)
	s.data[key] = &StorageItem{
		Value:     encoded,
		ExpiresAt: expiresAt,
	}
}

func (s *Storage) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[key]; exists {
		delete(s.data, key)
		return true
	}
	return false
}

func (s *Storage) All() map[string]*StorageItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*StorageItem)
	for k, v := range s.data {
		if v.ExpiresAt > 0 && v.ExpiresAt < time.Now().UnixMilli() {
			continue
		}
		result[k] = v
	}
	return result
}

func (s *Storage) Load(data map[string]StorageItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = make(map[string]*StorageItem)
	for k, v := range data {
		if v.ExpiresAt > 0 && v.ExpiresAt < time.Now().UnixMilli() {
			continue
		}
		item := v
		s.data[k] = &item
	}
}

func (s *Storage) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

func (s *Storage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]*StorageItem)
}

func (s *Storage) Marshal() (string, error) {
	data := s.All()
	bytes, err := sonic.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *Storage) Unmarshal(data string) error {
	var items map[string]StorageItem
	if err := sonic.Unmarshal([]byte(data), &items); err != nil {
		return err
	}
	s.Load(items)
	return nil
}
