package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
)

func main() {
	// Create server instance
	server := NewServer()

	// Setup router
	router := mux.NewRouter()

	// Health check endpoint
	router.HandleFunc("/health", server.HealthCheck).Methods("GET")

	// Flight reservation endpoints
	router.HandleFunc("/api/flights", server.CreateFlight).Methods("POST")
	router.HandleFunc("/api/flights", server.ListFlights).Methods("GET")
	router.HandleFunc("/api/flights/{id}", server.GetFlight).Methods("GET")
	router.HandleFunc("/api/flights/{id}", server.DeleteFlight).Methods("DELETE")

	// Setup server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	addr := fmt.Sprintf(":%s", port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Handle graceful shutdown
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("Shutting down server...")
		server.updater.Stop()
		if err := httpServer.Close(); err != nil {
			log.Printf("Error closing server: %v\n", err)
		}
	}()

	// Start server
	log.Printf("Flight Reservation Mock Server starting on %s\n", addr)
	log.Println("Endpoints:")
	log.Println("  POST   /api/flights       - Create flight reservation")
	log.Println("  GET    /api/flights       - List all flights")
	log.Println("  GET    /api/flights/{id}  - Get flight by ID")
	log.Println("  DELETE /api/flights/{id}  - Cancel flight reservation")
	log.Println("  GET    /health            - Health check")
	log.Println()
	log.Println("Status progression:")
	log.Println("  T+0s:  confirmed         - Initial booking")
	log.Println("  T+20s: details_changed   - Flight details updated")
	log.Println("  T+40s: delayed_one_day   - Flight delayed due to incident")

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v\n", err)
	}

	log.Println("Server stopped")
}
