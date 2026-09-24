package inventory

import (
	"errors"
	"reflect"
	"testing"
)

func TestInventoryConstructorAndZeroValue(t *testing.T) {
	for name, inv := range map[string]*Inventory{"NewInventory": NewInventory(), "zero value": {}} {
		inv.Add("apple", 3)
		inv.Add("apple", 2)
		if got := inv.Count("apple"); got != 5 {
			t.Errorf("%s: Count = %d, want 5", name, got)
		}
		if got := inv.Count("pear"); got != 0 {
			t.Errorf("%s: Count(missing) = %d", name, got)
		}
	}
	var zero Inventory
	if got := zero.Items(); len(got) != 0 {
		t.Errorf("zero Items = %v", got)
	}
	if got := zero.Count("x"); got != 0 {
		t.Errorf("zero Count = %d", got)
	}
}

func TestInventoryRemove(t *testing.T) {
	inv := NewInventory()
	inv.Add("bolt", 4)
	inv.Add("nut", 1)
	if err := inv.Remove("bolt", 5); !errors.Is(err, ErrInsufficient) {
		t.Errorf("Remove too many = %v, want ErrInsufficient", err)
	}
	if inv.Count("bolt") != 4 {
		t.Errorf("failed Remove changed stock: %d", inv.Count("bolt"))
	}
	if err := inv.Remove("bolt", 4); err != nil {
		t.Errorf("Remove exact = %v", err)
	}
	if inv.Count("bolt") != 0 {
		t.Errorf("Count after removing all = %d", inv.Count("bolt"))
	}
	if got := inv.Items(); !reflect.DeepEqual(got, []string{"nut"}) {
		t.Errorf("Items after removing all bolts = %v, want [nut]", got)
	}
	if err := inv.Remove("ghost", 1); !errors.Is(err, ErrInsufficient) {
		t.Errorf("Remove unknown = %v, want ErrInsufficient", err)
	}
}
