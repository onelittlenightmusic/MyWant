package main

import (
	"fmt"
	"math"
)

func main() {
	fmt.Println("M/M/1 Queue Theory vs Empirical Results")
	fmt.Println("=======================================")
	
	// Parameters from latest memory dump
	generatorRate := 50.0      // Rate parameter from config (now correct)
	serviceTime := 0.01        // Mean service time
	empiricalWait := 23.443774 // Average wait time from memory dump
	
	// After the rate fix: Rate parameter is now 1/mean_inter_arrival_time
	// So arrival rate λ = generatorRate
	lambda := generatorRate
	
	// Service rate μ = 1/mean_service_time
	mu := 1.0 / serviceTime
	
	// Utilization ρ = λ/μ
	rho := lambda / mu
	
	fmt.Printf("System Parameters:\n")
	fmt.Printf("Generator Rate (λ): %.6f packets/time\n", lambda)
	fmt.Printf("Service Time: %.6f time/packet\n", serviceTime)
	fmt.Printf("Service Rate (μ): %.6f packets/time\n", mu)
	fmt.Printf("Utilization (ρ): %.6f\n", rho)
	fmt.Printf("\n")
	
	if rho >= 1.0 {
		fmt.Printf("⚠️  System is unstable (ρ ≥ 1)! Queue will grow without bound.\n")
		fmt.Printf("Theoretical wait time: ∞\n")
	} else {
		// Theoretical M/M/1 results
		// Average wait time in queue W_q = ρ / (μ * (1 - ρ))
		theoreticalWaitTime := rho / (mu * (1 - rho))
		
		// Average number in queue L_q = ρ² / (1 - ρ)
		theoreticalQueueLength := (rho * rho) / (1 - rho)
		
		// Average system time W = 1 / (μ - λ)
		theoreticalSystemTime := 1 / (mu - lambda)
		
		fmt.Printf("Theoretical M/M/1 Results:\n")
		fmt.Printf("Average wait time in queue (W_q): %.6f\n", theoreticalWaitTime)
		fmt.Printf("Average queue length (L_q): %.6f\n", theoreticalQueueLength)
		fmt.Printf("Average system time (W): %.6f\n", theoreticalSystemTime)
		fmt.Printf("\n")
		
		fmt.Printf("Comparison with Simulation:\n")
		fmt.Printf("Empirical wait time: %.6f\n", empiricalWait)
		fmt.Printf("Theoretical wait time: %.6f\n", theoreticalWaitTime)
		fmt.Printf("Ratio (Empirical/Theoretical): %.4f\n", empiricalWait/theoreticalWaitTime)
		
		relativeError := math.Abs(theoreticalWaitTime-empiricalWait) / theoreticalWaitTime * 100
		fmt.Printf("Relative error: %.2f%%\n", relativeError)
		
		fmt.Printf("\n")
		if relativeError < 5.0 {
			fmt.Printf("✅ Excellent agreement with M/M/1 theory!\n")
		} else if relativeError < 15.0 {
			fmt.Printf("✅ Good agreement with M/M/1 theory\n")
		} else {
			fmt.Printf("❌ Poor agreement - check implementation or assumptions\n")
		}
		
		// Additional insights
		fmt.Printf("\nM/M/1 Queue Insights:\n")
		fmt.Printf("• With ρ = %.3f, the system is %.1f%% utilized\n", rho, rho*100)
		fmt.Printf("• Each packet waits %.1fx longer than the service time\n", theoreticalWaitTime/serviceTime)
		fmt.Printf("• On average, %.2f packets are waiting in queue\n", theoreticalQueueLength)
	}
}