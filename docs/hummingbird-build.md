# Parallel Hummingbird build (ROSAENG-75338)

`Dockerfile.hummingbird` retains the existing boilerplate Go compiler stage and
uses the digest-pinned `hi/core-runtime:2.43-openssl-fips` runtime. The production
`Dockerfile` and `entrypoint.sh` are unchanged. No DNF stage is needed: the build
installs no additional RPMs, and the API does not launch external executables or
install packages at runtime.

The hardened image provides the API binary and a static Go entrypoint launcher.
The launcher replaces itself with the API using `syscall.Exec`, preserving
`start`, `APP_ENV=dev` (local database), forwarded arguments and signal delivery.
Only that launcher forces `CGO_ENABLED=0`; application compiler flags are retained.
The app-owned data directory is prepared in the compiler stage and copied with
UID 65532/group 0 ownership and group write permissions. No UBI libraries or
package-installed filesystem is copied into the Hummingbird image.

The additional push PipelineRun changes only its name, application/component
labels, output tag and Dockerfile parameter. It stays outside the production
component's snapshots. Renovate covers both Dockerfiles and requires review of
Hummingbird digest updates.

## Local checks

Verification uses Linux amd64 images under emulation on an arm64 Podman VM.
The runtime digest was resolved using:

```
skopeo inspect --override-os linux --override-arch amd64 --format '{{.Digest}}' \
  docker://registry.access.redhat.com/hi/core-runtime:2.43-openssl-fips
```

Result: `sha256:dda4ca9f205064c1391168713fae4dfe32057f3a1e619540db34f9d167b71c74`.
JSON and YAML parsing, `git diff --check`, production Dockerfile content hash
comparison, and `go test ./build/hummingbird-entrypoint` pass.

The FIPS-capable runtime does not by itself establish validated application FIPS
operation. FIPS-host testing, Konflux scanning, Enterprise Contract and release
gates remain external validation requirements.

Both local builds passed on 2026-11-09:

```
podman build --platform linux/amd64 --pull=never \
  --cpu-period=100000 --cpu-quota=100000 \
  -f Dockerfile.hummingbird -t rhobs-synthetics-api-hummingbird-test .
podman build --platform linux/amd64 --pull=never \
  --cpu-period=100000 --cpu-quota=100000 \
  -f Dockerfile -t rhobs-synthetics-api-production-test .
```

The quota limits local compilation memory/concurrency; it adds no image settings.
Initial unconstrained concurrent builds lost the Podman connection under resource
pressure; the constrained retry completed successfully. The production build
reused the identical application compilation layer.

- Hardened image: `1058ee1ae3c91f4f16588af1f6fdc8fc2dd094c49d92c1daf4c96aa3daf7de42`.
- Production image: `4f5fdfe1a21b1c3d387292cc23171d2ecf0dcca8800fc3b0b87e505773847df6`.
- Production Dockerfile Git blob remains `2f8fefd8587291ad1f0607929289a0d0efa42cc5`.

For each image, explicit `--entrypoint /home/rhobs/rhobs-synthetics-api` with
`--help` and the normal image entrypoint with `--help` passed. With `APP_ENV=dev`,
HTTP `/readyz` returned 200, `POST /probes` returned 201, and `GET /probes`
returned the new probe. Copying `/home/rhobs/data` from each running container
confirmed a persisted JSON file. Containers stopped gracefully. The hardened
smoke ran both with its default UID 65532 and with arbitrary UID/group `12345:0`.
The production smoke ran with its unchanged default user.

`file` identifies the API executable as dynamically linked with interpreter
`/lib64/ld-linux-x86-64.so.2`; its successful help and HTTP/storage runs empirically
verify compatibility with the hardened runtime. The added launcher is statically
linked. Running the hardened dynamic loader with `--list` resolves `libresolv.so.2`
and `libc.so.6` from `/lib64`, with no missing libraries. All runtime libraries
and trust certificates come from Hummingbird;
there are no downloaded CLI tarballs, extra runtime RPMs, or copied UBI libraries.

Build logs: `/tmp/rhobs-synthetics-api-{hummingbird,production}-build.log`.
Smoke logs: `/tmp/rhobs-synthetics-api-{hummingbird,production}-smoke.log`.
These logs are local verification artifacts, not repository inputs.
