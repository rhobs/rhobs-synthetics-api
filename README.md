# RHOBS Synthetic Monitoring API and Agent

This application provides a synthetic monitoring API to be used within the RHOBS ecosystem. It can be configured using command-line flags or a YAML config file.

---

## 🚀 Getting Started

Run the API server using:

```sh
./rhobs-synthetics-api start
```

## Configuration
This app uses Viper for configuration and supports:

* CLI flags
* YAML config file (--config)

### CLI Flags
Flag | Type | Default | Description
---|---|---|---
`--host` | string | `"0.0.0.0"` | Host address to bind the server
`--port`, `-p` | int | `8080` | Port to run the server on
`--read-timeout` | duration | `5s` | Max duration for reading the entire request
`--write-timeout` | duration | `10s` | Max duration before timing out writes
`--graceful-timeout` | duration | `15s` | Time allowed for graceful shutdown
`--database-engine` | string | `"etcd"` | Backend database engine (e.g., etcd, postgres)
`--log-level` | string | `"info"` | Log verbosity: debug, info (`debug`, `info`)
`--config` | string | `(none)` | Path to YAML config file

### Config File Example
The following is an example of a configuration file that can be used to setup this applicaton:
```
# Server binding
host: "0.0.0.0"
port: 8080

# Timeout settings
read_timeout: "5s"         # How long to wait while reading the request body
write_timeout: "10s"       # Maximum duration before timing out response writes
graceful_timeout: "15s"    # Time allowed for graceful shutdown

# Database configuration
database_engine: "etcd"    # Supported: etcd, postgres, mysql (as implemented)

# Logging
log_level: "info"          # Options: debug, info
```

Use the `--config` flag to specify the file to use
```sh
./rhobs-synthetics-api start --config /path/to/config.yaml
```
