package kv

import "sort"

// MemStore is an in-memory Store.
type MemStore struct {
	data map[string]string
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{data: make(map[string]string)}
}

func (m *MemStore) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *MemStore) Set(key, value string) error {
	m.data[key] = value
	return nil
}

func (m *MemStore) Delete(key string) error {
	if _, ok := m.data[key]; !ok {
		return ErrNotFound
	}
	delete(m.data, key)
	return nil
}

func (m *MemStore) Keys() []string {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var _ Store = (*MemStore)(nil)
