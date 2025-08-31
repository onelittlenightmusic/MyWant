package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// ExpRand64 generates exponentially distributed random numbers with improved precision
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
	
	fmt.Println("ExpRand64 Distribution Analysis")
	fmt.Println("==============================")
	
	// Test distribution properties
	n := 100000
	samples := make([]float64, n)
	sum := 0.0
	
	for i := 0; i < n; i++ {
		samples[i] = ExpRand64()
		sum += samples[i]
	}
	
	// Calculate empirical mean
	mean := sum / float64(n)
	
	// Calculate empirical variance
	varSum := 0.0
	for _, x := range samples {
		varSum += (x - mean) * (x - mean)
	}
	variance := varSum / float64(n-1)
	stdDev := math.Sqrt(variance)
	
	// Sort for percentiles
	sort.Float64s(samples)
	
	// Calculate percentiles
	p50 := samples[n/2]
	p90 := samples[int(0.9*float64(n))]
	p95 := samples[int(0.95*float64(n))]
	p99 := samples[int(0.99*float64(n))]
	
	fmt.Printf("Sample size: %d\n", n)
	fmt.Printf("Empirical mean: %.6f (theoretical: 1.000000)\n", mean)
	fmt.Printf("Empirical variance: %.6f (theoretical: 1.000000)\n", variance)
	fmt.Printf("Empirical std dev: %.6f (theoretical: 1.000000)\n", stdDev)
	fmt.Printf("Median (50%%): %.6f (theoretical: %.6f)\n", p50, math.Log(2))
	fmt.Printf("90th percentile: %.6f (theoretical: %.6f)\n", p90, -math.Log(0.1))
	fmt.Printf("95th percentile: %.6f (theoretical: %.6f)\n", p95, -math.Log(0.05))
	fmt.Printf("99th percentile: %.6f (theoretical: %.6f)\n", p99, -math.Log(0.01))
	
	fmt.Println("\nM/M/1 Queue Analysis")
	fmt.Println("====================")
	
	// Parameters from memory dump
	lambda := 1.0 / 0.04  // arrival rate (1/inter-arrival time)
	mu := 1.0 / 0.01      // service rate (1/service time)
	rho := lambda / mu    // utilization
	
	fmt.Printf("Arrival rate (λ): %.2f packets/unit time\n", lambda)
	fmt.Printf("Service rate (μ): %.2f packets/unit time\n", mu)
	fmt.Printf("Utilization (ρ): %.6f\n", rho)
	
	if rho >= 1.0 {
		fmt.Printf("⚠️  System is unstable (ρ ≥ 1)! Queue will grow without bound.\n")
	} else {
		// Theoretical M/M/1 results
		theoreticalAvgWait := rho / (mu * (1 - rho))
		theoreticalAvgQueueLength := (rho * rho) / (1 - rho)
		theoreticalAvgSystemTime := 1 / (mu - lambda)
		
		fmt.Printf("Theoretical avg wait time: %.6f\n", theoreticalAvgWait)
		fmt.Printf("Theoretical avg queue length: %.6f\n", theoreticalAvgQueueLength)
		fmt.Printf("Theoretical avg system time: %.6f\n", theoreticalAvgSystemTime)
		
		// Empirical results from memory dump
		empiricalAvgWait := 16.707178168983248
		
		fmt.Printf("\nComparison with Simulation:\n")
		fmt.Printf("Empirical avg wait time: %.6f\n", empiricalAvgWait)
		fmt.Printf("Theoretical vs Empirical ratio: %.4f\n", theoreticalAvgWait/empiricalAvgWait)
		
		relativeError := math.Abs(theoreticalAvgWait-empiricalAvgWait) / theoreticalAvgWait * 100
		fmt.Printf("Relative error: %.2f%%\n", relativeError)
		
		if relativeError < 5.0 {
			fmt.Printf("✅ Excellent agreement with theory!\n")
		} else if relativeError < 15.0 {
			fmt.Printf("✅ Good agreement with theory\n")
		} else {
			fmt.Printf("❌ Poor agreement with theory - check implementation\n")
		}
	}
}