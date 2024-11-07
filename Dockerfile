# Use the official Golang image as the base image
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container
WORKDIR /app/mysql-manager

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code into the container
COPY cmd/ ./cmd
COPY internal/ ./internal
# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o mysql-manager /app/mysql-manager/cmd/mysql-manager

# Use a minimal alpine image for the final stage
FROM alpine:latest

# Install necessary system dependencies
RUN apk --no-cache add ca-certificates

# Set the working directory
WORKDIR /root/

# Copy the built executable from the builder stage
COPY --from=builder /app/mysql-manager/mysql-manager /usr/bin/mysql-manager

# Expose the default metrics port
EXPOSE 9104

# Set a default environment variable for the metrics port
ENV METRICS_PORT=9104

# Command to run the executable
ENTRYPOINT ["mysql-manager"]
