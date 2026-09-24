// Package inventory tracks item counts.
package inventory

import (
	"errors"
	"sort"
)

// ErrInsufficient is returned when removing more than is in stock.
var ErrInsufficient = errors.New("inventory: insufficient stock")

// Inventory maps item names to counts. The zero value is ready to use.
type Inventory struct {
	items map[string]int
}

// NewInventory returns an empty Inventory.
func NewInventory() *Inventory {
	return &Inventory{items: make(map[string]int)}
}

// Add increases the count for name by n.
func (inv *Inventory) Add(name string, n int) {
	if inv.items == nil {
		inv.items = make(map[string]int)
	}
	inv.items[name] += n
}

// Remove decreases the count for name by n.
func (inv *Inventory) Remove(name string, n int) error {
	have := inv.items[name]
	if n > have {
		return ErrInsufficient
	}
	if n == have {
		delete(inv.items, name)
		return nil
	}
	inv.items[name] = have - n
	return nil
}

// Count returns how many of name are held.
func (inv *Inventory) Count(name string) int {
	return inv.items[name]
}

// Items returns the held item names, sorted.
func (inv *Inventory) Items() []string {
	out := make([]string, 0, len(inv.items))
	for k := range inv.items {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
