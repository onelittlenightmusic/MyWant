FROM golang:1.22-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o bin/mywant cmd/server/*.go

EXPOSE 8080

CMD ["./bin/mywant", "8080", "localhost"]
