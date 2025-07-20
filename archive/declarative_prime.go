// Package main provides prime number generation node implementations for the declarative chain framework.
// This module demonstrates the Sieve of Eratosthenes algorithm using the declarative system.
package main

import (
	"fmt"
	"gochain/chain"
)

// PrimeTuple represents a number flowing through the prime sieve pipeline.
// Each tuple carries a candidate number and end-of-stream marker.
type PrimeTuple struct {
	Num   int  // The candidate number (-1 indicates end-of-stream)
	Prime bool // Whether this number has been confirmed as prime
}

// isEnded checks if this tuple represents an end-of-stream marker.
// Tuples with negative Num values signal that no more numbers will follow.
func (t *PrimeTuple) isEnded() bool {
	return t.Num < 0
}

// createGeneratorFunc creates a number generator function that produces consecutive integers.
// The generator creates numbers from 'start' to 'end' for prime testing.
//
// Parameters:
//   - start: First number to generate (typically 2)
//   - end: Last number to generate (inclusive)
//
// Returns: A chain function that generates sequential numbers for prime testing
func createPrimeGeneratorFunc(start, end int) func(chain.Chan, chain.Chan) bool {
	current := start
	return func(_, out chain.Chan) bool {
		// Check if we've generated all requested numbers
		if current > end {
			// Send end-of-stream marker
			out <- PrimeTuple{-1, false}
			fmt.Printf("[END] Number generation complete\n")
			return true // Signal completion
		}
		
		// Generate next number
		out <- PrimeTuple{current, false}
		current++
		return false // Continue generating
	}
}

// createPrimeFilterFunc creates a prime filter function that removes multiples of a given prime.
// This implements one stage of the Sieve of Eratosthenes algorithm.
//
// Parameters:
//   - prime: The prime number whose multiples should be filtered out
//
// Returns: A chain function that filters out multiples of the given prime
func createPrimeFilterFunc(prime int) func(chain.Chan, chain.Chan) bool {
	return func(in, out chain.Chan) bool {
		t := (<-in).(PrimeTuple)
		
		// Handle end-of-stream marker
		if t.isEnded() {
			fmt.Printf("[FILTER] Prime %d filter complete\n", prime)
			out <- t // Forward end marker
			return true // Signal completion
		}
		
		// Filter logic: pass through numbers that are not multiples of this prime
		if t.Num%prime != 0 {
			out <- t // Pass through non-multiples
		}
		// Drop multiples (don't forward them)
		
		return false // Continue filtering
	}
}

// createPrimeDetectorFunc creates a prime detection function that identifies new primes.
// When a number passes through all existing filters, it must be prime.
//
// Returns: A chain function that detects and outputs prime numbers
func createPrimeDetectorFunc() func(chain.Chan, chain.Chan) bool {
	return func(in, out chain.Chan) bool {
		t := (<-in).(PrimeTuple)
		
		// Handle end-of-stream marker
		if t.isEnded() {
			fmt.Printf("[DETECTOR] Prime detection complete\n")
			out <- t // Forward end marker
			return true // Signal completion
		}
		
		// If a number reaches here, it's prime (survived all filters)
		fmt.Printf("Prime found: %d\n", t.Num)
		t.Prime = true
		out <- t
		
		return false // Continue detecting
	}
}

// createPrimeCollectorFunc creates a collector function that gathers detected primes.
// The collector maintains a list of found primes and can trigger creation of new filters.
//
// Returns: A chain function that collects primes and manages the sieve expansion
func createPrimeCollectorFunc() func(chain.Chan) bool {
	primes := []int{}
	return func(in chain.Chan) bool {
		t := (<-in).(PrimeTuple)
		
		// Handle end-of-stream marker
		if t.isEnded() {
			fmt.Printf("[COLLECTOR] Found %d primes: %v\n", len(primes), primes)
			return true // Signal completion
		}
		
		// Collect the prime
		if t.Prime {
			primes = append(primes, t.Num)
		}
		
		return false // Continue collecting
	}
}

// RegisterPrimeNodeTypes registers all prime sieve node types with the chain builder.
// This function demonstrates extending the declarative framework for prime number generation.
//
// Node types registered:
//   - "prime_generator": Generates consecutive numbers for testing
//   - "prime_filter": Filters out multiples of a specific prime
//   - "prime_detector": Identifies numbers that have survived all filters as prime
//   - "prime_collector": Collects and displays found primes
//
// Parameters:
//   - builder: ChainBuilder instance to register node types with
func RegisterPrimeNodeTypes(builder *ChainBuilder) {
	// Register number generator node type
	// Expected params: start (int), end (int)
	builder.RegisterNodeType("prime_generator", func(params map[string]interface{}) interface{} {
		// Handle both int and float64 for start parameter
		var start int
		switch v := params["start"].(type) {
		case int:
			start = v
		case float64:
			start = int(v)
		default:
			panic(fmt.Sprintf("Invalid type for start parameter: %T", v))
		}
		
		// Handle both int and float64 for end parameter
		var end int
		switch v := params["end"].(type) {
		case int:
			end = v
		case float64:
			end = int(v)
		default:
			panic(fmt.Sprintf("Invalid type for end parameter: %T", v))
		}
		
		return createPrimeGeneratorFunc(start, end)
	})
	
	// Register prime filter node type
	// Expected params: prime (int)
	builder.RegisterNodeType("prime_filter", func(params map[string]interface{}) interface{} {
		// Handle both int and float64 for prime parameter
		var prime int
		switch v := params["prime"].(type) {
		case int:
			prime = v
		case float64:
			prime = int(v)
		default:
			panic(fmt.Sprintf("Invalid type for prime parameter: %T", v))
		}
		return createPrimeFilterFunc(prime)
	})
	
	// Register prime detector node type
	// Expected params: none
	builder.RegisterNodeType("prime_detector", func(params map[string]interface{}) interface{} {
		return createPrimeDetectorFunc()
	})
	
	// Register prime collector node type
	// Expected params: none
	builder.RegisterNodeType("prime_collector", func(params map[string]interface{}) interface{} {
		return createPrimeCollectorFunc()
	})
}