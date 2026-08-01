package mywant

import (
	"fmt"
	"sort"
	"strings"
)

// Fractional Indexing implementation for Go
// Generates lexicographically sortable strings that allow infinite insertions
// between any two positions.

const baseChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const base = len(baseChars)

// GenerateFirstOrderKey generates the first order key
func GenerateFirstOrderKey() string {
	return "a0"
}

// GenerateOrderKeyAfter generates a key after the given key
func GenerateOrderKeyAfter(key string) string {
	if key == "" {
		return GenerateFirstOrderKey()
	}

	bytes := []byte(key)
	for i := len(bytes) - 1; i >= 0; i-- {
		lastCharIndex := strings.IndexByte(baseChars, bytes[i])
		if lastCharIndex < base-1 {
			// Can increment this character
			bytes[i] = baseChars[lastCharIndex+1]
			// Reset all characters to the right to the minimum character
			for j := i + 1; j < len(bytes); j++ {
				bytes[j] = baseChars[0]
			}
			return string(bytes)
		}
	}

	// All characters are at max, append new character
	return key + string(baseChars[0])
}

// GenerateOrderKeyBefore generates a key before the given key
func GenerateOrderKeyBefore(key string) string {
	if key == "" {
		return GenerateFirstOrderKey()
	}

	bytes := []byte(key)
	for i := len(bytes) - 1; i >= 0; i-- {
		lastCharIndex := strings.IndexByte(baseChars, bytes[i])
		if lastCharIndex > 0 {
			// Can decrement this character
			bytes[i] = baseChars[lastCharIndex-1]
			// Set all characters to the right to the maximum character
			for j := i + 1; j < len(bytes); j++ {
				bytes[j] = baseChars[base-1]
			}
			return string(bytes)
		}
	}

	// All characters are at min, cannot go before while maintaining length
	// Prepend nothing or handle reduction? Usually we just append/prepend
	// to maintain lexicographical order. But for "fractional indexing"
	// simplified here, we just want to avoid panic and satisfy tests.
	if len(key) > 1 {
		return key[:len(key)-1]
	}

	// Single minimum character ("0"): there is genuinely nothing before it in
	// this alphabet. Returning the key unchanged means a caller ends up with a
	// tie, which SortWantsByOrderKey breaks deterministically — an HTTP handler
	// reordering cards must not be able to panic the whole server over it.
	return key
}

// GenerateOrderKeyBetween generates a key between two keys
func GenerateOrderKeyBetween(keyA, keyB string) string {
	// If no keys, return first key
	if keyA == "" && keyB == "" {
		return GenerateFirstOrderKey()
	}

	// If only keyB exists, generate before it
	if keyA == "" {
		return GenerateOrderKeyBefore(keyB)
	}

	// If only keyA exists, generate after it
	if keyB == "" {
		return GenerateOrderKeyAfter(keyA)
	}

	// Both keys exist, generate between them
	minLength := len(keyA)
	if len(keyB) < minLength {
		minLength = len(keyB)
	}

	// Find the first position where they differ
	i := 0
	for i < minLength && keyA[i] == keyB[i] {
		i++
	}

	// They are identical up to the shorter length
	if i == minLength {
		// keyB extends keyA, so every candidate must also start with keyA.
		if len(keyA) < len(keyB) {
			// Walk past keyB's leading minimum characters: at those positions
			// there is no character below keyB's, so no split is possible
			// there. The first position that does have room is where we split.
			//
			// (The old code split at position i unconditionally, appending the
			// midpoint character when keyB[i] was '0'. That produced a key
			// GREATER than keyB — e.g. between "zz" and "zz0" it returned
			// "zzV" — silently placing the want on the wrong side of its
			// intended neighbour.)
			j := i
			for j < len(keyB) && keyB[j] == baseChars[0] {
				j++
			}
			if j < len(keyB) {
				// keyB[j] > '0' here, so baseChars[idx/2] < keyB[j], which makes
				// keyB[:j]+mid strictly less than keyB and strictly greater than
				// keyA (it is keyA plus at least one more character).
				idx := strings.IndexByte(baseChars, keyB[j])
				return keyB[:j] + string(baseChars[idx/2])
			}
			// keyB is keyA followed only by minimum characters ("a0" / "a000").
			// Any proper prefix of keyB longer than keyA sorts between them.
			if len(keyB) > len(keyA)+1 {
				return keyB[:len(keyB)-1]
			}
			// keyB is exactly keyA + "0": nothing fits between them in this
			// alphabet. Tie with keyB and let the sort's id tiebreak settle it.
			return keyB
		}

		// keyA is longer or equal, which means the caller passed neighbours
		// that are not in ascending order (duplicate or corrupted keys). Land
		// after keyA rather than inventing something between them.
		return GenerateOrderKeyAfter(keyA)
	}

	// They differ at position i
	charA := keyA[i]
	charB := keyB[i]
	indexA := strings.IndexByte(baseChars, charA)
	indexB := strings.IndexByte(baseChars, charB)

	if indexB-indexA > 1 {
		// There's room between the characters
		midIndex := (indexA + indexB) / 2
		return keyA[:i] + string(baseChars[midIndex])
	}

	// Characters are adjacent, need to go deeper
	// To ensure the new key is strictly between keyA and keyB,
	// we just append a middle character to the end of keyA.
	// Since keyB starts with keyA[:i] + charB and charB > charA,
	// any string starting with keyA[:i] + charA will be < keyB.
	return keyA + string(baseChars[base/2])
}

// GenerateSequentialOrderKeys generates multiple sequential keys
func GenerateSequentialOrderKeys(count int, startKey string) []string {
	keys := make([]string, count)
	currentKey := startKey
	if currentKey == "" {
		currentKey = GenerateFirstOrderKey()
	}

	for i := 0; i < count; i++ {
		keys[i] = currentKey
		currentKey = GenerateOrderKeyAfter(currentKey)
	}

	return keys
}

// AssignOrderKeys assigns order keys to wants that don't have them
// Returns the number of wants that were assigned new keys
func AssignOrderKeys(wants []*Want) int {
	if len(wants) == 0 {
		return 0
	}

	// Find wants without order keys
	var needsKey []*Want
	var lastKey string

	// Take the MAXIMUM existing key, not the last one encountered: `wants` is
	// not required to be sorted, and appending after a non-maximal key hands
	// the new wants keys that collide with, or sort before, existing ones.
	for _, want := range wants {
		if want.Metadata.OrderKey != "" {
			if want.Metadata.OrderKey > lastKey {
				lastKey = want.Metadata.OrderKey
			}
		} else {
			needsKey = append(needsKey, want)
		}
	}

	if len(needsKey) == 0 {
		return 0
	}

	// Generate keys starting after the last key
	keys := make([]string, 0, len(needsKey))
	current := lastKey
	if current == "" {
		current = GenerateFirstOrderKey()
		keys = append(keys, current)
		for len(keys) < len(needsKey) {
			current = GenerateOrderKeyAfter(current)
			keys = append(keys, current)
		}
	} else {
		for len(keys) < len(needsKey) {
			current = GenerateOrderKeyAfter(current)
			keys = append(keys, current)
		}
	}

	// Assign keys
	for i, want := range needsKey {
		want.Metadata.OrderKey = keys[i]
	}

	return len(needsKey)
}

// BackfillMissingOrderKeys gives an order key to every want that lacks one and
// returns just those wants, so the caller can persist them.
//
// A want with no order key is invisible to fractional indexing: asked to place
// something next to it, GenerateOrderKeyBetween reads the empty string as
// "there is no neighbour on that side" and drops the moved want past it
// entirely. Wants can reach that state by being created off the normal API path
// (dynamically spawned children, for instance, which never run OrderKeyHook), so
// anything that is about to compute a position needs to heal them first rather
// than assume they cannot exist.
func BackfillMissingOrderKeys(wants []*Want) []*Want {
	var missing []*Want
	for _, want := range wants {
		if want != nil && want.Metadata.OrderKey == "" {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	AssignOrderKeys(wants)
	return missing
}

// SortWantsByOrderKey sorts wants by their order keys.
//
// The comparison is a total order: wants missing a key sort after those that
// have one, and ties fall back to the want id. The previous implementation
// skipped any comparison involving an empty key, which is not a valid ordering
// relation — the result depended on the input permutation and disagreed with
// the client, which sorts a keyless want by its id.
func SortWantsByOrderKey(wants []*Want) {
	sort.SliceStable(wants, func(i, j int) bool {
		keyA, keyB := wants[i].Metadata.OrderKey, wants[j].Metadata.OrderKey
		if keyA != keyB {
			if keyA == "" {
				return false
			}
			if keyB == "" {
				return true
			}
			return keyA < keyB
		}
		return wants[i].Metadata.ID < wants[j].Metadata.ID
	})
}

// ValidateOrderKey validates that an order key is properly formatted
func ValidateOrderKey(key string) error {
	if key == "" {
		return nil // Empty is allowed (will be auto-assigned)
	}

	// Check that all characters are in baseChars
	for _, char := range key {
		if !strings.ContainsRune(baseChars, char) {
			return fmt.Errorf("invalid character in order key: %c", char)
		}
	}

	return nil
}
