# Use the official Golang image to create a build artifact.
# This is a multi-stage build, so we'll use a temporary image for the build process.
FROM quay.io/redhat-services-prod/openshift/boilerplate:image-v8.0.0 AS builder

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

# Create a non-root user with home directory
RUN groupadd -g 1001 rhobs && \
    useradd -u 1001 -g 1001 -m -d /home/rhobs -s /bin/sh rhobs

# Set working directory to user's home
WORKDIR /home/rhobs

# Copy the binary from the builder stage
COPY --from=builder /app/rhobs-synthetics-api ./

# Copy the entrypoint script
COPY entrypoint.sh ./

# Create a data directory for local storage if needed and set permissions
# Make files executable by group and others to support OpenShift arbitrary UIDs
RUN mkdir -p /home/rhobs/data && \
    chown -R rhobs:0 /home/rhobs && \
    chmod -R g=u /home/rhobs && \
    chmod +x ./entrypoint.sh ./rhobs-synthetics-api

# Expose port 8080 to the outside world.
# Ports below 1024 require root privileges.
EXPOSE 8080

# Switch to the non-root user
USER rhobs

# Use the entrypoint script to start the application
ENTRYPOINT ["./entrypoint.sh"]
