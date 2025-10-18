# Mock Flight Reservation Server

A mock server that simulates flight reservation management with automatic status progression over time.

## Features

- **CRUD Operations**: Create, Read, and Delete flight reservations
- **Automatic Status Updates**: Flight status changes automatically over time
- **Time-based Simulation**: Realistic flight status progression

## Status Progression Timeline

When a flight reservation is created, it automatically progresses through these statuses:

1. **T+0s** - `confirmed`: Flight reservation confirmed
2. **T+20s** - `details_changed`: Flight details have been changed (gate/aircraft)
3. **T+40s** - `delayed_one_day`: Flight delayed by one day due to airport incident

## API Endpoints

### Create Flight Reservation
```bash
POST /api/flights
Content-Type: application/json

{
  "flight_number": "AA123",
  "from": "New York",
  "to": "Los Angeles",
  "departure_time": "2025-10-17T08:00:00Z",
  "arrival_time": "2025-10-17T11:30:00Z"
}
```

Response (201 Created):
```json
{
  "id": "uuid",
  "flight_number": "AA123",
  "from": "New York",
  "to": "Los Angeles",
  "departure_time": "2025-10-17T08:00:00Z",
  "arrival_time": "2025-10-17T11:30:00Z",
  "status": "confirmed",
  "status_message": "Flight reservation confirmed",
  "created_at": "2025-10-16T21:37:10Z",
  "updated_at": "2025-10-16T21:37:10Z"
}
```

### Get Flight by ID
```bash
GET /api/flights/{id}
```

### List All Flights
```bash
GET /api/flights
```

### Cancel Flight Reservation
```bash
DELETE /api/flights/{id}
```

Response (200 OK):
```json
{
  "message": "Flight reservation cancelled successfully",
  "id": "uuid"
}
```

### Health Check
```bash
GET /health
```

## Building and Running

### Build
```bash
go build -o flight-server
```

### Run
```bash
./flight-server
```

Server starts on port 8081 by default. Set `PORT` environment variable to use a different port:
```bash
PORT=9090 ./flight-server
```

## Testing Example

```bash
# Create a flight
FLIGHT_ID=$(curl -s -X POST http://localhost:8081/api/flights \
  -H "Content-Type: application/json" \
  -d '{
    "flight_number": "AA123",
    "from": "New York",
    "to": "Los Angeles",
    "departure_time": "2025-10-17T08:00:00Z",
    "arrival_time": "2025-10-17T11:30:00Z"
  }' | jq -r '.id')

# Check status immediately (confirmed)
curl -s http://localhost:8081/api/flights/$FLIGHT_ID | jq '.status'

# Wait 25 seconds and check again (details_changed)
sleep 25
curl -s http://localhost:8081/api/flights/$FLIGHT_ID | jq '.status'

# Wait another 20 seconds (delayed_one_day)
sleep 20
curl -s http://localhost:8081/api/flights/$FLIGHT_ID | jq '.status'

# Cancel the flight
curl -s -X DELETE http://localhost:8081/api/flights/$FLIGHT_ID
```

## Integration with FlightAgent

This server is designed to be called by a FlightAgent in the MyWant system:

1. **FlightAgent** calls POST to create a reservation
2. **FlightAgent** periodically polls GET to check status changes
3. **FlightAgent** can call DELETE to cancel if needed
4. **FlightAgent** can create new reservations after cancellation

## Dependencies

- `github.com/gorilla/mux` - HTTP router
- `github.com/google/uuid` - UUID generation
