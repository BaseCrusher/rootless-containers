# traefik

[Traefik](https://traefik.io/) on `scratch` — the release binary, a CA bundle
and nothing else, plus an optional watcher that keeps Traefik's certificates in
sync with a directory another process writes to.

## How it differs from `traefik`

| | official image | this image |
| --- | --- | --- |
| Base | Alpine | `scratch` |
| User | `root` | uid/gid `65532` |
| Ports 80/443 | bound as root | bound via `cap_net_bind_service` on the binary |
| Working dir | `/` | `/home/nonroot` |
| PID 1 | Traefik | `container-supervisor` |
| Configuration | flags or `TRAEFIK_*` env vars | `TRAEFIK_*` env vars only |
| Shell, package manager, `/etc/passwd` | present | none |

Everything else is stock Traefik: same `TRAEFIK_*` env vars, same static and
dynamic configuration files.

### Traefik flags do not work as container arguments

The entrypoint is `container-supervisor`, not Traefik, so anything appended to
`docker run` (or a compose `command:`) goes to the supervisor and never reaches
Traefik. Every Traefik flag has an environment equivalent — uppercase it, drop
the leading dashes and replace `.` with `_`:

```yaml
# does nothing
command:
  - --entrypoints.web.address=:80

# use this instead
environment:
  TRAEFIK_ENTRYPOINTS_WEB_ADDRESS: ":80"
```

A static configuration file is `TRAEFIK_CONFIGFILE`, not `--configFile`.

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| Traefik | [traefik/traefik](https://github.com/traefik/traefik) | `TRAEFIK_VERSION` |
| container-supervisor | [container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` |
| certwatcher | `certwatcher/` in this folder | built from source with the image |

## Usage

```sh
docker run --rm -p 80:80 -p 443:443 \
  -v ./dynamic.yml:/home/nonroot/config/dynamic.yml:ro \
  -e TRAEFIK_ENTRYPOINTS_WEB_ADDRESS=:80 \
  -e TRAEFIK_PROVIDERS_FILE_FILENAME=config/dynamic.yml \
  ghcr.io/basecrusher/rootless-containers/traefik:v3.7.9
```

Traefik takes its whole static configuration from `TRAEFIK_*` environment
variables, so nothing has to be mounted for a basic run. To use a config file
instead, mount it and point Traefik at it — it is not read from
`/etc/traefik/traefik.yml` by default here, because that path does not exist in
the image:

```sh
docker run --rm -p 80:80 \
  -v ./traefik.yml:/home/nonroot/config/traefik.yml:ro \
  -e TRAEFIK_CONFIGFILE=/home/nonroot/config/traefik.yml \
  ghcr.io/basecrusher/rootless-containers/traefik:v3.7.9
```

`/home/nonroot` is the working directory, so relative paths in the
configuration (ACME storage, dynamic config files, plugin storage) resolve
there. `/home/nonroot/config` is an empty directory owned by uid 65532 to mount
those into; `/home/nonroot/config/dynamic` is a second one, empty unless
certwatcher is enabled. Mounting over `/home/nonroot` itself hides the binary
and the container will not start.

ACME storage has to be a writable, mounted path owned by uid 65532 — the image
has no writable layer worth persisting:

```sh
docker run --rm -p 80:80 -p 443:443 \
  -v acme:/home/nonroot/config \
  -e TRAEFIK_CERTIFICATESRESOLVERS_LE_ACME_STORAGE=config/acme.json \
  ...
```

### certwatcher — certificates from a directory

Traefik cannot load certificates from a directory; it only loads what a dynamic
configuration names. `certwatcher` closes that gap: it polls a directory of
certificates and rewrites a dynamic configuration listing every one it finds, so
a certificate issued or renewed by another process is picked up without a
restart.

It is **disabled by default**: `supervisor.yml` ships it as `enabled: false`, so
nothing runs but Traefik until you override that key at container start.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SUPERVISOR_PROCESSES__CERTWATCHER__ENABLED` | `false` | set to `true` to run certwatcher at all |
| `CERTWATCHER_CERTS_DIR` | — | directory to watch. Required once enabled; without it the container aborts |
| `CERTWATCHER_OUTPUT` | `/home/nonroot/config/dynamic/certs.yml` | generated dynamic configuration |
| `CERTWATCHER_TEMPLATE` | `/home/nonroot/certs.tmpl` | template used to render it |
| `CERTWATCHER_INTERVAL` | `5s` | how often the directory is scanned |

The directory is walked recursively. Every `<name>.pem` with a sibling
`<name>.key` becomes one certificate; anything else — `.json` metadata, a `.pem`
whose key is missing — is skipped. That is the layout the
[coredns](../coredns/) image's `acmednschallenge` plugin writes under its
`certificateStorageDisk` path, where `<name>` is the domain and the `.pem` holds
the chain and the key concatenated. Traefik reads only the certificate blocks
out of `certFile`, so pointing it at that combined file is fine.

```yaml
services:
  traefik:
    image: ghcr.io/basecrusher/rootless-containers/traefik:v3.7.9
    environment:
      SUPERVISOR_PROCESSES__CERTWATCHER__ENABLED: "true"
      CERTWATCHER_CERTS_DIR: /certs
      TRAEFIK_ENTRYPOINTS_WEBSECURE_ADDRESS: ":443"
      TRAEFIK_PROVIDERS_FILE_DIRECTORY: /home/nonroot/config/dynamic
      TRAEFIK_PROVIDERS_FILE_WATCH: "true"
    volumes:
      - certs:/certs:ro
    ports:
      - 443:443

  coredns:
    image: ghcr.io/basecrusher/rootless-containers/coredns:v1.14.6
    volumes:
      - certs:/certs

volumes:
  certs:
```

`TRAEFIK_PROVIDERS_FILE_WATCH` must be `true` and the provider must point at the
directory the file is written into — that is what makes the reload happen.
Renewals are the reason: the generated configuration names the same paths as
before, but Traefik's file provider reads `certFile`/`keyFile` into the
configuration *before* comparing it against the running one, so the new bytes
count as a change and the certificate is swapped in. Rewriting the file is
therefore the trigger for both new domains and renewals.

The private key must be readable by uid 65532. The plugin writes keys `600` by
default, owned by the CoreDNS user, which Traefik cannot read — issue them as
`certificateStorageDisk PATH 640 GROUP` with a group both containers share.

To generate something other than a plain certificate list, mount your own
[`text/template`](https://pkg.go.dev/text/template) over
`/home/nonroot/certs.tmpl`. It is executed with a slice of `{Cert, Key}` paths,
sorted:

```
tls:
  certificates:
{{- range . }}
    - certFile: {{ .Cert }}
      keyFile: {{ .Key }}
{{- end }}
```

### The Docker provider requires a socket proxy

Do not mount `/var/run/docker.sock` into this image. It is `root:root`, so uid
65532 cannot read it and `--providers.docker` fails with *permission denied* on
every retry — and the only ways to fix that directly (`--group-add` the socket's
gid, or running as root) hand the container root-equivalent control of the host,
which is the entire thing this image exists to avoid.

Put a read-only socket proxy in front of it instead and point Traefik at the
proxy over TCP. Use
[docker-socket-proxy-go](https://github.com/BaseCrusher/docker-socket-proxy-go) —
the companion image to this one, read-only by default with every API section
denied until you enable it:

```yaml
services:
  dockerproxy:
    image: ghcr.io/basecrusher/docker-socket-proxy-go:latest
    environment:
      CONTAINERS: "true"
      EVENTS: "true"
      VERSION: "true"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks: [internal]

  traefik:
    image: ghcr.io/basecrusher/rootless-containers/traefik:v3.7.9
    environment:
      TRAEFIK_ENTRYPOINTS_WEB_ADDRESS: ":80"
      TRAEFIK_PROVIDERS_DOCKER: "true"
      TRAEFIK_PROVIDERS_DOCKER_ENDPOINT: tcp://dockerproxy:2375
      TRAEFIK_PROVIDERS_DOCKER_EXPOSEDBYDEFAULT: "false"
    ports:
      - 80:80
    networks: [internal]

networks:
  internal:
```

Traefik needs `VERSION` (API handshake), `CONTAINERS` (discovery) and `EVENTS`
(live reconfiguration); add `SERVICES` and `TASKS` for Swarm. Everything else
stays denied and `READONLY` defaults to `true`, so a compromised Traefik cannot
start containers or reach the rest of the Docker API. The proxy is the only
thing touching the socket, and it never has to run in this image.

### Scratch caveats

- **No shell.** `docker exec` and shell-form health checks do not work; use
  `docker logs`, Traefik's `ping` and `metrics` endpoints. There is no `-debug`
  flavour — the base is always `scratch`.
- **Log lines are prefixed.** container-supervisor labels each process's output,
  so `docker logs` shows `[traefik    ] …` and `[certwatcher] …` rather than
  stock Traefik format. Anything parsing the logs has to strip the prefix.
- **No timezone database.** Go falls back to UTC, so logs and time-based
  middleware are in UTC regardless of `TZ`.
- **No `/etc/passwd`.** The container runs as numeric uid/gid `65532`, which is
  all Traefik needs; anything resolving the user by name sees none.
- `/etc/ssl/certs/ca-certificates.crt` is present, so ACME and TLS to backends
  verify normally.

### Ports

- `80/tcp`, `443/tcp`

Any port Traefik is configured to listen on works, privileged ones included —
`cap_net_bind_service` is a file capability on the binary, so low ports bind
without root.

## Images

Every push to `main` touching this folder publishes all three platforms:

| Tag | Base | Notes |
| --- | --- | --- |
| `:v3.7.9`, `:latest` | `scratch` | the only flavour |

All under `ghcr.io/basecrusher/rootless-containers/traefik`. The version tag is
`TRAEFIK_VERSION` from `docker-bake.hcl`, which is also the release that gets
downloaded, so the two can't drift; `latest` follows `main`.

The workflow lints the Dockerfile with droast before building and scans the
pushed image with Trivy afterwards, failing on any fixable `HIGH` or `CRITICAL`
vulnerability.

## Cross-platform builds

```sh
docker buildx bake -f ./traefik/docker-bake.hcl
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `traefik` | `${REGISTRY}/traefik:${TRAEFIK_VERSION}`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY` and `TRAEFIK_VERSION` are bake variables — override either from the
environment (`TRAEFIK_VERSION=v3.7.8 docker buildx bake …`).

The upstream release tarball is named
`traefik_<version>_linux_<arch>.tar.gz`, where `<arch>` is
`$TARGETARCH$TARGETVARIANT` — `amd64`, `arm64`, `armv7`. container-supervisor
publishes its assets under the same suffix. Neither is emulated: the download
stage runs on `$BUILDPLATFORM` and only unpacks.

`certwatcher` is the one thing built rather than downloaded. Its stage is
pinned to `$BUILDPLATFORM` too and Go is pointed at
`$TARGETOS`/`$TARGETARCH`/`$TARGETVARIANT`, so it cross-compiles without
emulation as well. Adding a platform to `docker-bake.hcl` is enough as long as
Traefik and container-supervisor both publish a binary for it.
