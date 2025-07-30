#!/bin/sh

# Default arguments
ARGS="start"

# Check if APP_ENV is set to "dev"
if [ "$APP_ENV" = "dev" ]; then
  ARGS="$ARGS --database-engine local"
fi

# Execute the main application with the determined arguments
# The "$@" allows passing additional arguments from the 'docker run' command
exec ./rhobs-synthetics-api $ARGS "$@"
