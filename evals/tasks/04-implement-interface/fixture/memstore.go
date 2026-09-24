package kv

// MemStore is an in-memory Store.
type MemStore struct{}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{}
}

func (m *MemStore) Get(key string) (string, error) { return "", ErrNotImplemented }
func (m *MemStore) Set(key, value string) error    { return ErrNotImplemented }
func (m *MemStore) Delete(key string) error        { return ErrNotImplemented }
func (m *MemStore) Keys() []string                 { return nil }

var _ Store = (*MemStore)(nil)
