package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "external-agent-grpc/proto"
	"google.golang.org/grpc"
)

// AgentServer implements the gRPC AgentService
type AgentServer struct {
	pb.UnimplementedAgentServiceServer
}

// Execute implements DoAgent execution via gRPC
func (s *AgentServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	log.Printf("[gRPC DoAgent] Executing agent %s for want %s", req.AgentName, req.WantId)
	log.Printf("[gRPC DoAgent] Operation: %s", req.Operation)
	log.Printf("[gRPC DoAgent] Want state: %+v", req.WantState)

	startTime := time.Now()

	// Simulate flight booking process
	time.Sleep(1 * time.Second)

	// Generate booking result
	bookingID := fmt.Sprintf("GRPC-FLT-%d", time.Now().Unix())
	stateUpdates := map[string]string{
		"flight_booking_id": bookingID,
		"booking_status":    "confirmed",
		"booking_time":      time.Now().Format(time.RFC3339),
		"booking_method":    "gRPC",
	}

	// Add original state fields if present
	if departure, ok := req.WantState["departure"]; ok {
		stateUpdates["departure"] = departure
	}
	if arrival, ok := req.WantState["arrival"]; ok {
		stateUpdates["arrival"] = arrival
	}

	executionTime := time.Since(startTime).Milliseconds()

	response := &pb.ExecuteResponse{
		Status:          "completed",
		StateUpdates:    stateUpdates,
		ExecutionTimeMs: executionTime,
	}

	log.Printf("[gRPC DoAgent] Flight booked: %s (took %dms)", bookingID, executionTime)
	return response, nil
}

// StartMonitor implements MonitorAgent start via gRPC
func (s *AgentServer) StartMonitor(ctx context.Context, req *pb.MonitorRequest) (*pb.MonitorResponse, error) {
	log.Printf("[gRPC MonitorAgent] Starting monitor for want %s", req.WantId)
	log.Printf("[gRPC MonitorAgent] Agent: %s", req.AgentName)
	log.Printf("[gRPC MonitorAgent] Callback URL: %s", req.CallbackUrl)

	monitorID := fmt.Sprintf("grpc-monitor-%d", time.Now().UnixNano())

	// Start background monitoring (simplified - in production this would be more sophisticated)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		checkCount := 0
		for {
			select {
			case <-ctx.Done():
				log.Printf("[gRPC MonitorAgent] Context cancelled for monitor %s", monitorID)
				return
			case <-ticker.C:
				checkCount++
				log.Printf("[gRPC MonitorAgent] Check #%d for monitor %s", checkCount, monitorID)

				// Simulate detecting state change after 3 checks
				if checkCount >= 3 {
					log.Printf("[gRPC MonitorAgent] State change detected for monitor %s", monitorID)
					// In real implementation, would send callback to MyWant
					return
				}
			}
		}
	}()

	response := &pb.MonitorResponse{
		MonitorId: monitorID,
		Status:    "started",
	}

	log.Printf("[gRPC MonitorAgent] Monitor started: %s", monitorID)
	return response, nil
}

// StopMonitor implements MonitorAgent stop via gRPC
func (s *AgentServer) StopMonitor(ctx context.Context, req *pb.StopMonitorRequest) (*pb.StopMonitorResponse, error) {
	log.Printf("[gRPC MonitorAgent] Stopping monitor %s", req.MonitorId)

	response := &pb.StopMonitorResponse{
		Status: "stopped",
	}

	log.Printf("[gRPC MonitorAgent] Monitor stopped: %s", req.MonitorId)
	return response, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9001"
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAgentServiceServer(grpcServer, &AgentServer{})

	log.Printf("External gRPC Agent Server starting on port %s", port)
	log.Printf("Available services:")
	log.Printf("  - AgentService.Execute (DoAgent)")
	log.Printf("  - AgentService.StartMonitor (MonitorAgent)")
	log.Printf("  - AgentService.StopMonitor (MonitorAgent)")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
