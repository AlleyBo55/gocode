package buddy

import (
	"math"
)

// Rarity tiers for companions.
const (
	Common    = "common"
	Uncommon  = "uncommon"
	Rare      = "rare"
	Epic      = "epic"
	Legendary = "legendary"
)

// Rarities in order, with cumulative weights summing to 100.
var Rarities = []string{Common, Uncommon, Rare, Epic, Legendary}

// RarityWeights maps each rarity to its percentage chance.
var RarityWeights = map[string]float64{
	Common:    60,
	Uncommon:  25,
	Rare:      10,
	Epic:      4,
	Legendary: 1,
}

// Stats tracks a companion's three attributes.
type Stats struct {
	Debugging int
	Chaos     int
	Snark     int
}

// Companion is a terminal buddy with deterministic traits.
type Companion struct {
	Name    string
	Species string
	Rarity  string
	Stats   Stats
	Soul    string // user-given personality blurb
}

// Species list — at least 10 species with simple ASCII art.
var Species = []string{
	"duck", "cat", "ghost", "robot", "owl",
	"blob", "dragon", "penguin", "snail", "mushroom",
	"octopus", "cactus",
}

// Mulberry32 returns a deterministic PRNG function seeded with the given value.
// This is a direct port of the Mulberry32 algorithm from Claude Code's
// companion.ts. Each call to the returned function produces the next uint32.
func Mulberry32(seed uint32) func() uint32 {
	a := seed
	return func() uint32 {
		a += 0x6D2B79F5
		t := a
		t = (t ^ (t >> 15)) * (1 | a)
		t = (t + (t^(t>>7))*(61|t)) ^ t
		return t ^ (t >> 14)
	}
}

// mulberry32Float wraps Mulberry32 to return floats in [0, 1).
func mulberry32Float(seed uint32) func() float64 {
	rng := Mulberry32(seed)
	return func() float64 {
		return float64(rng()) / 4294967296.0
	}
}

// hashString produces a 32-bit FNV-1a hash of s (matching Claude Code's hashString).
func hashString(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

const salt = "friend-2026-401"

// rollRarity picks a rarity tier using weighted random selection.
func rollRarity(rng func() float64) string {
	total := 0.0
	for _, w := range RarityWeights {
		total += w
	}
	roll := rng() * total
	for _, r := range Rarities {
		roll -= RarityWeights[r]
		if roll < 0 {
			return r
		}
	}
	return Common
}

// pick selects a random element from a slice.
func pick(rng func() float64, items []string) string {
	return items[int(math.Floor(rng()*float64(len(items))))]
}

// Stat names for rolling.
var statNames = []string{"debugging", "chaos", "snark"}

var rarityFloor = map[string]int{
	Common:    5,
	Uncommon:  15,
	Rare:      25,
	Epic:      35,
	Legendary: 50,
}

// rollStats generates stats with one peak, one dump, rest scattered.
func rollStats(rng func() float64, rarity string) Stats {
	floor := rarityFloor[rarity]
	peak := pick(rng, statNames)
	dump := pick(rng, statNames)
	for dump == peak {
		dump = pick(rng, statNames)
	}

	vals := make(map[string]int)
	for _, name := range statNames {
		if name == peak {
			v := floor + 50 + int(math.Floor(rng()*30))
			if v > 100 {
				v = 100
			}
			vals[name] = v
		} else if name == dump {
			v := floor - 10 + int(math.Floor(rng()*15))
			if v < 1 {
				v = 1
			}
			vals[name] = v
		} else {
			vals[name] = floor + int(math.Floor(rng()*40))
		}
	}
	return Stats{
		Debugging: vals["debugging"],
		Chaos:     vals["chaos"],
		Snark:     vals["snark"],
	}
}

// RollCompanion deterministically generates a companion from a user ID.
// The same userID always produces the same companion.
func RollCompanion(userID string) *Companion {
	key := userID + salt
	rng := mulberry32Float(hashString(key))

	rarity := rollRarity(rng)
	species := pick(rng, Species)
	stats := rollStats(rng, rarity)

	return &Companion{
		Name:    species + "-buddy",
		Species: species,
		Rarity:  rarity,
		Stats:   stats,
	}
}
