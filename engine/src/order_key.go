package mywant

import (
	"fmt"
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

	// Increment the last character
	lastChar := key[len(key)-1]
	lastCharIndex := strings.IndexByte(baseChars, lastChar)

	if lastCharIndex < base-1 {
		// Can increment the last character
		return key[:len(key)-1] + string(baseChars[lastCharIndex+1])
	}

	// Last character is at max, append new character
	return key + "0"
}

// GenerateOrderKeyBefore generates a key before the given key
func GenerateOrderKeyBefore(key string) string {
	if key == "" {
		return GenerateFirstOrderKey()
	}

	// Decrement the last character
	lastChar := key[len(key)-1]
	lastCharIndex := strings.IndexByte(baseChars, lastChar)

	if lastCharIndex > 0 {
		// Can decrement the last character
		return key[:len(key)-1] + string(baseChars[lastCharIndex-1])
	}

	// Need to go to previous "digit"
	if len(key) == 1 {
		// Can't go before single character - this is the minimum
		panic("Cannot generate key before minimum key")
	}

	prefix := key[:len(key)-1]
	return GenerateOrderKeyBefore(prefix)
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
		// If keyA is shorter, we can append to it
		if len(keyA) < len(keyB) {
			nextChar := keyB[i]
			nextCharIndex := strings.IndexByte(baseChars, nextChar)

			if nextCharIndex > 0 {
				// Can insert between by using a character before nextChar
				midCharIndex := nextCharIndex / 2
				return keyA + string(baseChars[midCharIndex])
			}

			// nextChar is '0', need to append to keyA
			return keyA + string(baseChars[base/2])
		}

		// keyA is longer or equal, append to keyA
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
	midCharIndex := base / 2
	return keyA[:i+1] + string(baseChars[midCharIndex])
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

	// First, find the last assigned key (for appending)
	for _, want := range wants {
		if want.Metadata.OrderKey != "" {
			lastKey = want.Metadata.OrderKey
		} else {
			needsKey = append(needsKey, want)
		}
	}

	if len(needsKey) == 0 {
		return 0
	}

	// Generate keys starting after the last key
	keys := GenerateSequentialOrderKeys(len(needsKey), lastKey)
	if lastKey != "" {
		// Skip the first key since it would be the same as lastKey
		keys = keys[1:]
		// Generate one more
		keys = append(keys, GenerateOrderKeyAfter(keys[len(keys)-1]))
	}

	// Assign keys
	for i, want := range needsKey {
		want.Metadata.OrderKey = keys[i]
	}

	return len(needsKey)
}

// SortWantsByOrderKey sorts wants by their order keys
func SortWantsByOrderKey(wants []*Want) {
	// Simple bubble sort for now (can optimize later if needed)
	n := len(wants)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			keyA := wants[j].Metadata.OrderKey
			keyB := wants[j+1].Metadata.OrderKey

			// If either key is empty, maintain current order
			if keyA == "" || keyB == "" {
				continue
			}

			if strings.Compare(keyA, keyB) > 0 {
				wants[j], wants[j+1] = wants[j+1], wants[j]
			}
		}
	}
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
