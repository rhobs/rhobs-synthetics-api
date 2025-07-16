# Use the official Golang image to create a build artifact.
# This is a multi-stage build, so we'll use a temporary image for the build process.
FROM golang:1.24 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
RUN make build

# Start a new, fresh image to reduce the final image size.
# ubi-minimal is a lightweight image from Red Hat.
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

WORKDIR /

# Copy the binary from the builder stage
COPY --from=builder /app/rhobs-synthetics-api .

# Copy the entrypoint script
COPY entrypoint.sh .

# Set permissions for the non-root user.
# We create a user with UID 1001. OpenShift runs containers with arbitrary UIDs by default,
# so this is a good practice. We assign the user to the root group (GID 0)
# and give ownership of the app directory and binary to the new user.
RUN chmod +x ./entrypoint.sh && chown 1001:0 ./entrypoint.sh && chown 1001:0 ./rhobs-synthetics-api

# Expose port 8080 to the outside world.
# Ports below 1024 require root privileges.
EXPOSE 8080

# Switch to the non-root user
USER 1001

# Use the entrypoint script to start the application
ENTRYPOINT ["./entrypoint.sh"]
