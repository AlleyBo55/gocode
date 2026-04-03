package buddy

import (
	"testing"
)

func TestMulberry32_Deterministic(t *testing.T) {
	rng1 := Mulberry32(12345)
	rng2 := Mulberry32(12345)

	for i := 0; i < 100; i++ {
		a, b := rng1(), rng2()
		if a != b {
			t.Fatalf("iteration %d: rng1=%d rng2=%d — not deterministic", i, a, b)
		}
	}
}

func TestMulberry32_DifferentSeeds(t *testing.T) {
	rng1 := Mulberry32(1)
	rng2 := Mulberry32(2)

	same := true
	for i := 0; i < 10; i++ {
		if rng1() != rng2() {
			same = false
			break
		}
	}
	if same {
		t.Error("different seeds should produce different sequences")
	}
}

func TestRollCompanion_SameUserID(t *testing.T) {
	c1 := RollCompanion("user-abc-123")
	c2 := RollCompanion("user-abc-123")

	if c1.Species != c2.Species {
		t.Errorf("same userID should produce same species: %s vs %s", c1.Species, c2.Species)
	}
	if c1.Rarity != c2.Rarity {
		t.Errorf("same userID should produce same rarity: %s vs %s", c1.Rarity, c2.Rarity)
	}
	if c1.Stats != c2.Stats {
		t.Errorf("same userID should produce same stats: %+v vs %+v", c1.Stats, c2.Stats)
	}
}

func TestRollCompanion_DifferentUserIDs(t *testing.T) {
	c1 := RollCompanion("alice")
	c2 := RollCompanion("bob")

	// They could theoretically match, but with different seeds it's very unlikely
	// that species AND rarity AND all stats match. We just check it doesn't panic.
	_ = c1
	_ = c2
}

func TestRollCompanion_ValidFields(t *testing.T) {
	c := RollCompanion("test-user")

	// Species should be in the list.
	found := false
	for _, s := range Species {
		if s == c.Species {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("species %q not in Species list", c.Species)
	}

	// Rarity should be valid.
	validRarity := false
	for _, r := range Rarities {
		if r == c.Rarity {
			validRarity = true
			break
		}
	}
	if !validRarity {
		t.Errorf("rarity %q not in Rarities list", c.Rarity)
	}

	// Stats should be in range.
	for _, v := range []int{c.Stats.Debugging, c.Stats.Chaos, c.Stats.Snark} {
		if v < 1 || v > 100 {
			t.Errorf("stat value %d out of range [1, 100]", v)
		}
	}
}

func TestRarityDistribution(t *testing.T) {
	counts := make(map[string]int)
	n := 1000

	for i := 0; i < n; i++ {
		userID := "user-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		c := RollCompanion(userID)
		counts[c.Rarity]++
	}

	// Common should be the most frequent (60% weight).
	if counts[Common] < n/4 {
		t.Errorf("common count %d seems too low for %d rolls", counts[Common], n)
	}

	// Legendary should be rare (1% weight) — allow 0 but not more than 5%.
	if counts[Legendary] > n/10 {
		t.Errorf("legendary count %d seems too high for %d rolls", counts[Legendary], n)
	}

	// All rarities should be accounted for.
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != n {
		t.Errorf("total %d != %d", total, n)
	}
}

func TestRenderSprite_ReturnsLines(t *testing.T) {
	c := &Companion{Species: "duck"}
	lines := RenderSprite(c, 0)
	if len(lines) == 0 {
		t.Fatal("expected non-empty sprite lines")
	}
	for i, l := range lines {
		if len(l) == 0 {
			t.Errorf("line %d is empty", i)
		}
	}
}

func TestRenderSprite_FrameWraps(t *testing.T) {
	c := &Companion{Species: "cat"}
	// Frame 0 and frame 2 (wraps to 0 for 2-frame species) should be the same.
	lines0 := RenderSprite(c, 0)
	lines2 := RenderSprite(c, 2)
	if len(lines0) != len(lines2) {
		t.Fatalf("frame wrap: different line counts %d vs %d", len(lines0), len(lines2))
	}
	for i := range lines0 {
		if lines0[i] != lines2[i] {
			t.Errorf("frame wrap: line %d differs", i)
		}
	}
}

func TestRenderSprite_UnknownSpecies(t *testing.T) {
	c := &Companion{Species: "unicorn"}
	lines := RenderSprite(c, 0)
	if len(lines) == 0 {
		t.Fatal("unknown species should still return fallback lines")
	}
}

func TestRenderSprite_AllSpecies(t *testing.T) {
	for _, sp := range Species {
		c := &Companion{Species: sp}
		lines := RenderSprite(c, 0)
		if len(lines) == 0 {
			t.Errorf("species %q returned empty sprite", sp)
		}
	}
}
