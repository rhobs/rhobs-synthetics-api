# RHOBS Synthetic Monitoring API and Agent

This application provides a synthetic monitoring API to be used within the RHOBS ecosystem. It can be configured using command-line flags or a YAML config file.

---

## Building Binary

Build the `rhobs-synthetics-api` binary:

```sh
make build
```

## Building Local Image

Build the `rhobs-synthetics-api` binary and local container image:

```sh
make docker-build
```

## Running

Run the API server locally using:
```sh
./rhobs-synthetics-api start --kubeconfig ~/.kube/config --namespace rhobs
```
(This assumes you're authenticated to a cluster. Running a cluster using something like `kind` works well.)

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
`--database-engine` | string | `"etcd"` | Backend database engine (e.g., etcd, local)
`--data-dir` | string | `"data"` | Directory for local storage (only valid with --database-engine=local)
`--log-level` | string | `"info"` | Log verbosity: debug, info (`debug`, `info`)
`--config` | string | `(none)` | Path to YAML config file
`--kubeconfig` | string | `(none)` | Path to kubeconfig file (optional, for out-of-cluster development)
`--namespace` | string | `default` | The Kubernetes namespace to store probe configmaps in.

### Config File Example
The following is an example of a configuration file that can be used to setup this application:
```
# Server binding
host: "0.0.0.0"
port: 8080

# Timeout settings
read_timeout: "5s"         # How long to wait while reading the request body
write_timeout: "10s"       # Maximum duration before timing out response writes
graceful_timeout: "15s"    # Time allowed for graceful shutdown

# Kubernetes configuration
kubeconfig: "/path/to/your/kubeconfig" # Optional, for out-of-cluster development
namespace: "my-probes-namespace"     # Namespace to store probe configmaps

# Database configuration
database_engine: "etcd"    # Supported: etcd, local
data_dir: "/path/to/data"  # Directory for local storage (only used with 'local' engine)

# Logging
log_level: "info"          # Options: debug, info
```

Use the `--config` flag to specify the file to use
```sh
./rhobs-synthetics-api start --config /path/to/config.yaml
```

## Running with Docker

You can build and run this application in a Docker container.

### Building the Image

```sh
make docker-build
```

### Running the Container

**Development Mode (using in-memory local store):**

By setting the `APP_ENV=dev` environment variable, the container will automatically start the server using the `local` database engine, which is useful for local testing without needing Kubernetes.

```sh
docker run -p 8080:8080 -e APP_ENV=dev rhobs-synthetics-api
```

## Example Commands

### Create a Probe

Create an example probe config:
```
$ curl -s -X POST http://localhost:8080/probes \
-H 'Content-Type: application/json' \
-d '{
  "static_url": "https://api.mycluster.example.com/livez",
  "labels": {
    "cluster-id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
    "management-cluster-id": "8e0a074c-f1e3-4957-be75-425e611142e4",
    "private": "false"
  }
}'
```

This will create a ConfigMap like this:
```
$ oc get cm probe-config-0cc7648a-751e-4e65-9365-a3d01d5ee21e -o yaml

apiVersion: v1
data:
  probe-config.json: '{"id":"0cc7648a-751e-4e65-9365-a3d01d5ee21e","labels":{"cluster-id":"d290f1ee-6c54-4b01-90e6-d701748f0851","management-cluster-id":"8e0a074c-f1e3-4957-be75-425e611142e4","private":"false"},"static_url":"https://api.mycluster.example.com/livez"}'
kind: ConfigMap
metadata:
  creationTimestamp: "2025-07-08T17:34:07Z"
  labels:
    app: rhobs-synthetics
    cluster-id: d290f1ee-6c54-4b01-90e6-d701748f0851
    management-cluster-id: 8e0a074c-f1e3-4957-be75-425e611142e4
    private: "false"
    rhobs-synthetics/static-url-hash: 0920a2aca3a5c7a722f348f6623d3494541cad934cc246219d47903a3d1741e
  name: probe-config-0cc7648a-751e-4e65-9365-a3d01d5ee21e
  namespace: default
  resourceVersion: "7369"
  uid: 2ef5adc1-bfee-4bfc-a9ad-b2477a1178c2
```

### List Probes

**Get all probes**
```
$ curl -s 'http://localhost:8080/probes' | jq

{
  "probes": [
    {
      "id": "176937a9-a1bb-4163-b602-a1416abe2f3c",
      "labels": {
        "cluster-id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
        "management-cluster-id": "8e0a074c-f1e3-4957-be75-425e611142e4",
        "private": "false"
      },
      "static_url": "https://api.mycluster.example.com/livez"
    },
    {
      "id": "f19f36b6-20bd-4576-bf81-6904e299f98c",
      "labels": {
        "cluster-id": "d290f1ee-6c54-4b01-90e6-d701748f0852",
        "management-cluster-id": "8e0a074c-f1e3-4957-be75-425e611142e4",
        "private": "true"
      },
      "static_url": "https://api2.mycluster.example.com/livez"
    }
  ]
}
```

**Get all probes using filters**
```
$ curl -s 'http://localhost:8080/probes?label_selector=private=false,management-cluster-id=8e0a074c-f1e3-4957-be75-425e611142e4' | jq

{
  "probes": [
    {
      "id": "176937a9-a1bb-4163-b602-a1416abe2f3c",
      "labels": {
        "cluster-id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
        "management-cluster-id": "8e0a074c-f1e3-4957-be75-425e611142e4",
        "private": "false"
      },
      "static_url": "https://api.mycluster.example.com/livez"
    }
  ]
}
```

**Get single probe by ID**
```
$ curl -s 'http://localhost:8080/probes/176937a9-a1bb-4163-b602-a1416abe2f3c' | jq

{
  "id": "176937a9-a1bb-4163-b602-a1416abe2f3c",
  "labels": {
    "cluster-id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
    "management-cluster-id": "8e0a074c-f1e3-4957-be75-425e611142e4",
    "private": "false"
  },
  "static_url": "https://api.mycluster.example.com/livez"
}
```

**Update a probe by ID**
```
curl -s -X PATCH -H "Content-Type: application/json" \
  -d '{"status": "active"}' \
  http://localhost:8080/probes/06581d72-ce30-4ff6-a761-6b0b972257cc | jq
{
  "id": "06581d72-ce30-4ff6-a761-6b0b972257cc",
  "labels": {
    "cluster-id": "d290f1ee-6c54-4b01-90e6-d701748f0852",
    "management-cluster-id": "8e0a074c-f1e3-4957-be75-425e611142e4",
    "private": "false"
  },
  "static_url": "https://api.mycluster.example.com/livez",
  "status": "active"
}
```

## Delete Probes

** Delete single probe by ID**
```
$ curl -s -X DELETE http://localhost:8080/probes/176937a9-a1bb-4163-b602-a1416abe2f3c
```

## OpenShift Deployment Templates

This repository includes OpenShift templates for deploying the synthetics API in OpenShift environments:

### Templates

* `templates/synthetics-api-template.yaml` - Main deployment template containing:
  - Service (service on port 8080)
  - ServiceAccount
  - StatefulSet with resource limits and requests

* `templates/service-monitor-synthetics-api-template.yaml` - Prometheus monitoring template containing:
  - ServiceMonitor for metrics collection on `/metrics` endpoint

### Deploying with OpenShift Templates

To deploy the synthetics API using the OpenShift templates:

```sh
# Deploy the main application
oc process -f templates/synthetics-api-template.yaml \
  -p IMAGE_TAG=latest \
  -p NAMESPACE=rhobs | oc apply -f -

# Deploy the service monitor for metrics
oc process -f templates/service-monitor-synthetics-api-template.yaml \
  -p NAMESPACE=rhobs | oc apply -f -
```

### Template Parameters

**synthetics-api-template.yaml:**
- `IMAGE_TAG` - Container image tag (default: latest)
- `NAMESPACE` - Target namespace (default: rhobs)

**service-monitor-synthetics-api-template.yaml:**
- `IMAGE_TAG` - Container image tag (default: latest)
- `NAMESPACE` - Target namespace for both the application and ServiceMonitor (default: rhobs)
