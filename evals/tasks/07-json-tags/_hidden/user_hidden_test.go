package user

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUserJSONKeys(t *testing.T) {
	u := User{ID: 7, FullName: "Ada Lovelace", Email: "ada@example.com", CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), IsAdmin: true}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":7,"full_name":"Ada Lovelace","email":"ada@example.com","created_at":"2024-01-02T03:04:05Z","is_admin":true}`
	if string(data) != want {
		t.Errorf("got  %s\nwant %s", data, want)
	}

	var back User
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != u {
		t.Errorf("round trip changed the value: %+v", back)
	}
}

func TestUserEmptyEmailIsOmitted(t *testing.T) {
	data, err := json.Marshal(User{ID: 1, FullName: "No Mail"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, present := m["email"]; present {
		t.Errorf("empty email should be omitted: %s", data)
	}
	for _, k := range []string{"id", "full_name", "created_at", "is_admin"} {
		if _, present := m[k]; !present {
			t.Errorf("key %q missing: %s", k, data)
		}
	}
}
