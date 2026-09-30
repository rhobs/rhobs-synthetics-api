# AGENTS.md

Providing guidance to coding agents when working with code in this repository.

## What this is

A Go HTTP API that stores synthetic-monitoring probe definitions (a URL plus labels) for the RHOBS ecosystem. It doesn't run probes itself. Other components talk to it: RMO creates probes, refreshes a heartbeat on them, and deletes them. The synthetics agent reads probes, creates Probe CRs, and reports status back through `PATCH`.

## Commands

```sh
make build                      # builds ./rhobs-synthetics-api from ./cmd/api/main.go
make test                       # go test -cover ./...
make test-templates             # only the OpenShift template tests in ./templates
make lint                       # golangci-lint (auto-installs pinned version into GOPATH/bin)
make lint-fix
make generate                   # regenerate pkg/apis/v1/types.go from api/v1/openapi.yaml
make coverage                   # hack/codecov.sh; writes coverage.out (excludes pkg/apis/v1)
make docker-build               # uses podman by default; override with CONTAINER_ENGINE=docker

go test ./internal/probestore -run TestKubernetesProbeStore -v   # single package / test
```

Run locally without a cluster (file-backed store, writes JSON files to `./data`):

```sh
./rhobs-synthetics-api start --database-engine local
```

Run against a cluster (the default `etcd` engine, which really means Kubernetes ConfigMaps):

```sh
./rhobs-synthetics-api start --kubeconfig ~/.kube/config --namespace rhobs
```

In the container image, `APP_ENV=dev` makes `entrypoint.sh` add `--database-engine local`. The Swagger UI is served at `/docs`, the spec at `/api/v1/openapi.json`, and Prometheus metrics at `/metrics`.

## Architecture

**The API is spec-first.** `api/v1/openapi.yaml` is the source of truth. `build/codegen/generate.go` has a `//go:generate` directive that runs oapi-codegen with `build/codegen/cfg.yaml` and produces `pkg/apis/v1/types.go`, which contains the models, the strict-server interface, the std-http router and the embedded spec. Don't edit `types.go` by hand: change the spec, run `make generate`, then implement any new operations on `api.Server`.

**Request path** (`cmd/api/main.go`): API requests enter the outer `http.ServeMux` at `/`, then pass through `metrics.Middleware`, `OapiRequestValidator` (which validates against the embedded spec), and the generated `HandlerFromMux`. Because metrics wraps validation, it records validator responses such as `400` errors. `/livez`, `/readyz`, `/docs`, `/api/v1/openapi.json`, and `/metrics` are registered directly on the outer mux and bypass this API middleware chain. `/readyz` checks Kubernetes API connectivity only when the `etcd` engine is in use.

**Handlers** (`internal/api/server.go`): `Server` implements the generated `StrictServerInterface`. Expected client errors (400/403/404/409) are returned as typed response objects. Returning a Go `error` produces a 500. Not-found detection uses `k8serrors.IsNotFound` for both backends; the local store deliberately returns k8s NotFound errors.

**Storage** (`internal/probestore`): the `ProbeStorage` interface has two implementations.
- `KubernetesProbeStore` (engine `etcd`): each probe is a ConfigMap named `probe-config-<uuid>`, with the probe JSON under the `probe-config.json` data key. Probe labels are copied onto the ConfigMap labels so that the `label_selector` query param becomes a Kubernetes label selector. The exception is the `last-reconciled` heartbeat, which is stored as an **annotation** to avoid Prometheus label churn; `UpdateProbe` migrates old label-based values.
- `LocalProbeStore` (engine `local`): one JSON file per probe. It's for development only, and its GC is a no-op.

**System-managed labels:** the app writes `app=rhobs-synthetics-probe`, `rhobs-synthetics/status` and `rhobs-synthetics/static-url-hash` (the SHA-256 of `static_url`, truncated to 63 characters). `private` is also protected. `PATCH` returns 403 if a client tries to add or change any of them. These constants are duplicated in `internal/api` and `internal/probestore/constants.go`, so keep the two in sync. Duplicate detection uses the URL hash, but probes in `terminating` or `failed` status don't block creating a new probe for the same URL.

**Probe lifecycle** (status: `pending` → `active` / `failed` → `terminating` → deleted):
- `DELETE` deletes `pending`/`failed` probes right away. It moves an `active` probe to `terminating` so the agent can remove its Probe CR first.
- `PATCH` with `status: deleted` removes the storage immediately. The agent uses this to finish cleanup.
- Two background goroutines start alongside the server. `MonitorProbes` updates the probes-total gauge every minute, grouped by status and `private`. `GarbageCollectProbes` runs every `ProbeTerminationGracePeriod` (15m). GC marks a ConfigMap as terminating by changing its `rhobs-synthetics/status` label and adding the `rhobs-synthetics/termination-started-at` annotation. It does not update the status inside `probe-config.json`. GC makes this transition when the heartbeat is older than `PROBE_STALE_TTL` (default 15m), or when no heartbeat was ever received and the probe is older than `PROBE_UNLABELED_TTL` (default 24h). A probe is deleted only after it has been terminating for the full grace period. That delete uses a resourceVersion precondition because multiple API replicas run GC concurrently. Preserve that behavior when you change GC.

**Configuration:** Cobra + Viper. Flags use kebab-case, and Viper keys and YAML config keys use snake_case (for example `--read-timeout` ↔ `read_timeout`). The `NAMESPACE` env var binds to `namespace`. The only other env vars are the two GC TTLs, which are read directly with `os.Getenv`. The `--data-dir` flag is declared but never bound to Viper, so it is silently ignored and the local store falls back to `./data`. Only `data_dir` in a YAML config file takes effect, and startup rejects it unless `database_engine` is `local`.

## Deployment artifacts

- `templates/` holds the OpenShift Templates used for real deployment: Service, SA/Role/RoleBinding, NetworkPolicy, a multi-replica Deployment with anti-affinity, and ServiceMonitor. `templates/templates_test.go` asserts their structure, including HA and NetworkPolicy details, so update the tests whenever you change a template.
- `config/` contains plain manifests (Deployment, Service, ServiceAccount, Role, and RoleBinding). Its namespaced Role grants access to ConfigMaps, Leases, and Events, and the store assumes the namespace already exists. The OpenShift template Role currently grants access to ConfigMaps only.
- `test/smoke-template.yaml` is a smoke-check Job template. `.tekton/` holds the Konflux pipelines. The builder image tag in `Dockerfile` must match the one in `.ci-operator.yaml`.
