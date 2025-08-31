package main

import (
	"fmt"
	"math"
)

func main() {
	fmt.Println("Debug Output Analysis")
	fmt.Println("====================")
	
	// Sample from recent debug output
	arrivals := []float64{
		1.634570,  // Packet 1
		2.056378,  // Packet 2  
		2.459514,  // Packet 3
		2.557842,  // Packet 4
		2.685213,  // Packet 5
		2.979749,  // Packet 6
		4.623221,  // Packet 7
		5.794324,  // Packet 8
		7.134634,  // Packet 9
		7.744873,  // Packet 10
	}
	
	waitTimes := []float64{
		0.000000,  // Packet 1
		0.261426,  // Packet 2
		0.742864,  // Packet 3
		2.174594,  // Packet 4
		2.611798,  // Packet 5
		5.088590,  // Packet 6
		3.632292,  // Packet 7
		3.710151,  // Packet 8
		3.008738,  // Packet 9
		5.336114,  // Packet 10
	}
	
	serviceTimes := []float64{
		0.683235,  // Packet 1
		0.884574,  // Packet 2
		1.530058,  // Packet 3
		0.564575,  // Packet 4
		2.771327,  // Packet 5
		0.187174,  // Packet 6
		1.248963,  // Packet 7
		0.638896,  // Packet 8
		2.937615,  // Packet 9
		0.103738,  // Packet 10
	}
	
	// Calculate inter-arrival times
	fmt.Println("Inter-arrival Time Analysis:")
	fmt.Println("============================")
	
	interArrivalTimes := make([]float64, len(arrivals)-1)
	for i := 1; i < len(arrivals); i++ {
		interArrivalTimes[i-1] = arrivals[i] - arrivals[i-1]
	}
	
	// Calculate statistics
	sumInterArrival := 0.0
	for _, iat := range interArrivalTimes {
		sumInterArrival += iat
		fmt.Printf("Inter-arrival: %.6f\n", iat)
	}
	meanInterArrival := sumInterArrival / float64(len(interArrivalTimes))
	empiricalArrivalRate := 1.0 / meanInterArrival
	
	fmt.Printf("\nMean inter-arrival time: %.6f\n", meanInterArrival)
	fmt.Printf("Empirical arrival rate: %.6f packets/time\n", empiricalArrivalRate)
	fmt.Printf("Configured rate parameter: 50.0\n")
	fmt.Printf("Rate parameter interpretation: %.6f (1/meanInterArrival)\n", 1.0/meanInterArrival)
	
	// Service time analysis
	fmt.Println("\nService Time Analysis:")
	fmt.Println("======================")
	
	sumServiceTime := 0.0
	for _, st := range serviceTimes {
		sumServiceTime += st
		fmt.Printf("Service time: %.6f\n", st)
	}
	meanServiceTime := sumServiceTime / float64(len(serviceTimes))
	empiricalServiceRate := 1.0 / meanServiceTime
	
	fmt.Printf("\nMean service time: %.6f\n", meanServiceTime)
	fmt.Printf("Empirical service rate: %.6f packets/time\n", empiricalServiceRate)
	fmt.Printf("Configured service_time parameter: 0.01\n")
	
	// Wait time analysis
	fmt.Println("\nWait Time Analysis:")
	fmt.Println("===================")
	
	sumWaitTime := 0.0
	for _, wt := range waitTimes {
		sumWaitTime += wt
		fmt.Printf("Wait time: %.6f\n", wt)
	}
	meanWaitTime := sumWaitTime / float64(len(waitTimes))
	
	fmt.Printf("\nMean wait time (first 10 packets): %.6f\n", meanWaitTime)
	
	// Compare with M/M/1 theory using empirical rates
	fmt.Println("\nM/M/1 Theory with Empirical Rates:")
	fmt.Println("===================================")
	
	lambda := empiricalArrivalRate
	mu := empiricalServiceRate  
	rho := lambda / mu
	
	if rho >= 1.0 {
		fmt.Printf("System is unstable (ρ = %.3f ≥ 1)\n", rho)
	} else {
		theoreticalWaitTime := rho / (mu * (1 - rho))
		fmt.Printf("λ (arrival rate): %.6f\n", lambda)
		fmt.Printf("μ (service rate): %.6f\n", mu)
		fmt.Printf("ρ (utilization): %.6f\n", rho)
		fmt.Printf("Theoretical wait time: %.6f\n", theoreticalWaitTime)
		fmt.Printf("Empirical wait time: %.6f\n", meanWaitTime)
		fmt.Printf("Ratio: %.2f\n", meanWaitTime/theoreticalWaitTime)
	}
	
	// Check rate parameter issue
	fmt.Println("\nRate Parameter Investigation:")
	fmt.Println("=============================")
	
	expectedInterArrivalTime := 1.0 / 50.0  // If rate=50 means 50 packets/time
	fmt.Printf("Expected inter-arrival time (rate=50): %.6f\n", expectedInterArrivalTime)
	fmt.Printf("Actual mean inter-arrival time: %.6f\n", meanInterArrival)
	fmt.Printf("Ratio (Actual/Expected): %.2f\n", meanInterArrival/expectedInterArrivalTime)
	
	if math.Abs(meanInterArrival - expectedInterArrivalTime) > 0.001 {
		fmt.Printf("❌ Rate parameter is NOT working as expected!\n")
		fmt.Printf("Rate=50 should give inter-arrival time ≈ 0.02, but got %.3f\n", meanInterArrival)
	} else {
		fmt.Printf("✅ Rate parameter working correctly\n")
	}
}