# Build stage
FROM golang:1.23.2-alpine AS builder

WORKDIR /app

# Install necessary build tools
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/guac-ai-mole ./cmd/server/main.go

# Final stage
FROM alpine:3.19

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/guac-ai-mole .
COPY --from=builder /app/web/static ./web/static

# Create non-root user
RUN adduser -D appuser
USER appuser

# Expose port
EXPOSE 8000

# Set environment variables
ENV GUACAIMOLE_OPENAI_API_KEY=""

# Command to run
ENTRYPOINT ["./guac-ai-mole"]