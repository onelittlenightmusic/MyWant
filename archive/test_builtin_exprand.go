package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// ExpRand64 generates exponentially distributed random numbers with improved precision (for comparison)
func ExpRand64() float64 {
	u := rand.Float64()
	if u == 0.0 {
		u = math.SmallestNonzeroFloat64
	} else if u == 1.0 {
		u = 1.0 - math.SmallestNonzeroFloat64
	}
	return -math.Log(u)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	fmt.Println("Built-in rand.ExpFloat64() vs Custom ExpRand64() Distribution Analysis")
	fmt.Println("=====================================================================")

	// Test both implementations
	n := 100000
	builtinSamples := make([]float64, n)
	customSamples := make([]float64, n)

	// Generate samples
	for i := 0; i < n; i++ {
		builtinSamples[i] = rand.ExpFloat64()
		customSamples[i] = ExpRand64()
	}

	// Analyze built-in implementation
	fmt.Println("\nBuilt-in rand.ExpFloat64() Results:")
	fmt.Println("===================================")
	analyzeDistribution(builtinSamples)

	// Analyze custom implementation
	fmt.Println("\nCustom ExpRand64() Results:")
	fmt.Println("===========================")
	analyzeDistribution(customSamples)

	// Direct comparison
	fmt.Println("\nDirect Comparison:")
	fmt.Println("==================")
	compareImplementations(builtinSamples, customSamples)
}

func analyzeDistribution(samples []float64) {
	n := len(samples)

	// Calculate empirical mean
	sum := 0.0
	for _, x := range samples {
		sum += x
	}
	mean := sum / float64(n)

	// Calculate empirical variance
	varSum := 0.0
	for _, x := range samples {
		varSum += (x - mean) * (x - mean)
	}
	variance := varSum / float64(n-1)
	stdDev := math.Sqrt(variance)

	// Sort for percentiles
	sortedSamples := make([]float64, n)
	copy(sortedSamples, samples)
	sort.Float64s(sortedSamples)

	// Calculate percentiles
	p50 := sortedSamples[n/2]
	p90 := sortedSamples[int(0.9*float64(n))]
	p95 := sortedSamples[int(0.95*float64(n))]
	p99 := sortedSamples[int(0.99*float64(n))]

	fmt.Printf("Sample size: %d\n", n)
	fmt.Printf("Empirical mean: %.6f (theoretical: 1.000000, error: %.2f%%)\n", mean, math.Abs(mean-1.0)*100)
	fmt.Printf("Empirical variance: %.6f (theoretical: 1.000000, error: %.2f%%)\n", variance, math.Abs(variance-1.0)*100)
	fmt.Printf("Empirical std dev: %.6f (theoretical: 1.000000, error: %.2f%%)\n", stdDev, math.Abs(stdDev-1.0)*100)

	theoreticalMedian := math.Log(2)
	fmt.Printf("Median (50%%): %.6f (theoretical: %.6f, error: %.2f%%)\n", p50, theoreticalMedian, math.Abs(p50-theoreticalMedian)/theoreticalMedian*100)

	theoretical90 := -math.Log(0.1)
	fmt.Printf("90th percentile: %.6f (theoretical: %.6f, error: %.2f%%)\n", p90, theoretical90, math.Abs(p90-theoretical90)/theoretical90*100)

	theoretical95 := -math.Log(0.05)
	fmt.Printf("95th percentile: %.6f (theoretical: %.6f, error: %.2f%%)\n", p95, theoretical95, math.Abs(p95-theoretical95)/theoretical95*100)

	theoretical99 := -math.Log(0.01)
	fmt.Printf("99th percentile: %.6f (theoretical: %.6f, error: %.2f%%)\n", p99, theoretical99, math.Abs(p99-theoretical99)/theoretical99*100)

	// Overall quality assessment
	meanError := math.Abs(mean-1.0) * 100
	varError := math.Abs(variance-1.0) * 100
	medianError := math.Abs(p50-theoreticalMedian) / theoreticalMedian * 100

	avgError := (meanError + varError + medianError) / 3
	fmt.Printf("Average error: %.2f%%\n", avgError)

	if avgError < 1.0 {
		fmt.Printf("✅ Excellent distribution quality\n")
	} else if avgError < 3.0 {
		fmt.Printf("✅ Good distribution quality\n")
	} else {
		fmt.Printf("⚠️  Fair distribution quality\n")
	}
}

func compareImplementations(builtin, custom []float64) {
	n := len(builtin)

	// Calculate means
	builtinMean := 0.0
	customMean := 0.0
	for i := 0; i < n; i++ {
		builtinMean += builtin[i]
		customMean += custom[i]
	}
	builtinMean /= float64(n)
	customMean /= float64(n)

	// Calculate variances
	builtinVar := 0.0
	customVar := 0.0
	for i := 0; i < n; i++ {
		builtinVar += (builtin[i] - builtinMean) * (builtin[i] - builtinMean)
		customVar += (custom[i] - customMean) * (custom[i] - customMean)
	}
	builtinVar /= float64(n - 1)
	customVar /= float64(n - 1)

	fmt.Printf("Mean difference: %.6f (builtin: %.6f, custom: %.6f)\n",
		math.Abs(builtinMean-customMean), builtinMean, customMean)
	fmt.Printf("Variance difference: %.6f (builtin: %.6f, custom: %.6f)\n",
		math.Abs(builtinVar-customVar), builtinVar, customVar)

	// Kolmogorov-Smirnov test approximation
	builtinSorted := make([]float64, n)
	customSorted := make([]float64, n)
	copy(builtinSorted, builtin)
	copy(customSorted, custom)
	sort.Float64s(builtinSorted)
	sort.Float64s(customSorted)

	maxDiff := 0.0
	for i := 0; i < n; i++ {
		diff := math.Abs(builtinSorted[i] - customSorted[i])
		if diff > maxDiff {
			maxDiff = diff
		}
	}

	fmt.Printf("Maximum sample difference: %.6f\n", maxDiff)

	if maxDiff < 0.1 {
		fmt.Printf("✅ Implementations produce very similar distributions\n")
	} else if maxDiff < 0.5 {
		fmt.Printf("✅ Implementations produce similar distributions\n")
	} else {
		fmt.Printf("⚠️  Implementations show noticeable differences\n")
	}
}
