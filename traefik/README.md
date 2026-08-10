# traefik

[Traefik](https://traefik.io/) on `scratch` — the release binary, a CA bundle
and nothing else, plus an optional watcher that keeps Traefik's certificates in
sync with a directory another process writes to and an optional exporter that
ships the access log to [CrowdSec](https://www.crowdsec.net/).

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
| access-log-exporter | `access-log-exporter/` in this folder | built from source with the image |

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
nothing runs but Traefik until you override that key at container start. It is a
`type: ticker` process — container-supervisor runs it once at startup and then
every 5 seconds, and each run scans, writes if needed, and exits. There is no
filesystem watch: a change is detected by content, not by an inotify event or a
timestamp, so a rewritten-but-identical certificate does not cause a reload and
no event can be missed.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SUPERVISOR_PROCESSES__CERTWATCHER__ENABLED` | `false` | set to `true` to run certwatcher at all |
| `SUPERVISOR_PROCESSES__CERTWATCHER__TICKER` | `@every 5s` | how often the directory is scanned. The `@every` prefix is mandatory |
| `CERTWATCHER_CERTS_DIR` | — | directory to watch. Required once enabled; without it the container aborts |
| `CERTWATCHER_OUTPUT` | `/home/nonroot/config/dynamic/certs.yml` | generated dynamic configuration |
| `CERTWATCHER_TEMPLATE` | `/home/nonroot/certs.tmpl` | template used to render it |

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

Which is why the file is only rewritten when something actually changed. Each run
hashes every certificate's path and contents into one fingerprint and stamps it
into the first line of the output as a YAML comment:

```yaml
# certwatcher 4f3c…
tls:
  certificates:
```

The generated file is its own state — the next run compares that line against
what it just scanned and exits without writing when they match. A renewal
changes the certificate's bytes but not its path, so hashing the contents is
what makes it visible; nothing else in the pipeline would notice. Delete or edit
that line and the next run rewrites the file.

`CERTWATCHER_OUTPUT` has to land in a directory uid 65532 can write to, so
certwatcher and a read-only mount of your own dynamic configuration cannot share
one directory: mounted over `/home/nonroot/config/dynamic` read-only,
certwatcher logs a write failure on every run and no certificate is ever served
— it does not abort, so watch for that line. Either mount that volume
read-write and give uid 65532 write access to it, or keep the two apart — write
`certs.yml` to `/home/nonroot/config` and mount the read-only configuration on
`/home/nonroot/config/dynamic` below it:

```yaml
      CERTWATCHER_OUTPUT: /home/nonroot/config/certs.yml
      TRAEFIK_PROVIDERS_FILE_DIRECTORY: /home/nonroot/config
    volumes:
      - ./dynamic:/home/nonroot/config/dynamic:ro
```

The file provider reads the directory recursively, so both are picked up. It
parses every `.yml`, `.yaml` and `.toml` it finds there — `acme.json` is ignored,
but a static `traefik.yml` mounted into the same directory would be read as
dynamic configuration; keep it somewhere else.

The private key must be readable by uid 65532. The plugin writes keys `600` by
default, owned by the CoreDNS user, which Traefik cannot read — issue them as
`certificateStorageDisk PATH 640 GROUP` with a group both containers share.

To generate something other than a plain certificate list, mount your own
[`text/template`](https://pkg.go.dev/text/template) over
`/home/nonroot/certs.tmpl`. It is executed with a slice of `{Cert, Key}` paths,
sorted, and its output goes below the fingerprint line, which certwatcher writes
itself:

```
tls:
  certificates:
{{- range . }}
    - certFile: {{ .Cert }}
      keyFile: {{ .Key }}
{{- end }}
```

### access-log-exporter — access logs to CrowdSec

`access-log-exporter` ships Traefik's access log to CrowdSec's
[`http` datasource](https://docs.crowdsec.net/docs/data_sources/http/) — the
[crowdsec](../crowdsec/) image in this repo, or any other: every 5
seconds it reads the lines added since the last run and POSTs them as
newline-delimited JSON. CrowdSec's `crowdsecurity/traefik-logs` parser reads
them as-is.

It is **disabled by default**, and pushing over HTTP rather than letting
CrowdSec acquire the log itself is deliberate: `source: docker` needs the Docker
socket, which is Swarm-only and not something this image will mount, and
`source: file` needs a shared volume whose topology differs between Swarm and
Kubernetes. A POST to a service address is byte-for-byte the same deployment on
both, needs no volume, and several Traefik replicas need no coordination —
each keeps its own offset beside its own log file.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SUPERVISOR_PROCESSES__ACCESSLOGEXPORTER__ENABLED` | `false` | set to `true` to run access-log-exporter at all |
| `SUPERVISOR_PROCESSES__ACCESSLOGEXPORTER__TICKER` | `@every 5s` | how often it runs. The `@every` prefix is mandatory |
| `ACCESSLOGEXPORTER_URL` | — | POST target. `http://user:pass@host:8081/traefik` sends basic auth. Required once enabled; without it the process aborts |
| `ACCESSLOGEXPORTER_FILE` | `/home/nonroot/config/access.log` | access log to read. Must match `TRAEFIK_ACCESSLOG_FILEPATH` |
| `ACCESSLOGEXPORTER_STATE` | `<FILE>.offset` | where the byte offset is kept |
| `ACCESSLOGEXPORTER_BATCH` | `1000` | lines per request; one run loops until it reaches the end of the file |
| `ACCESSLOGEXPORTER_MAX_SIZE` | `67108864` (64 MiB) | truncate the log above this many bytes; `0` truncates every run, `-1` never truncates |
| `ACCESSLOGEXPORTER_HEADERS` | — | extra headers, one `Key: Value` per line — see [Multiple headers](#multiple-headers) |
| `ACCESSLOGEXPORTER_CONTENT_TYPE` | `application/x-ndjson` | request content type |
| `ACCESSLOGEXPORTER_ECHO` | `false` | also print the lines it reads to stdout |

Traefik has to write the access log to a **file** in **JSON**: stdout belongs to
container-supervisor, so a sibling process cannot read it, and the CrowdSec
parser only accepts lines starting with `{`.

```yaml
services:
  traefik:
    image: ghcr.io/basecrusher/rootless-containers/traefik:v3.7.9
    environment:
      SUPERVISOR_PROCESSES__ACCESSLOGEXPORTER__ENABLED: "true"
      ACCESSLOGEXPORTER_URL: http://traefik:change-me@crowdsec:8081/traefik
      TRAEFIK_ACCESSLOG_FILEPATH: /home/nonroot/config/access.log
      TRAEFIK_ACCESSLOG_FORMAT: json
      TRAEFIK_ENTRYPOINTS_WEB_ADDRESS: ":80"
    ports:
      - 80:80

  crowdsec:
    image: ghcr.io/basecrusher/rootless-containers/crowdsec:v1.7.8
    environment:
      SUPERVISOR_PROCESSES__CSCLI__ENABLED: "true"
      SUPERVISOR_PROCESSES__CSCLI__ARGUMENTS: collections install crowdsecurity/traefik
    volumes:
      - ./acquis.yaml:/etc/crowdsec/acquis.d/traefik.yaml:ro
      - csdata:/var/lib/crowdsec/data

volumes:
  csdata:
```

```yaml
# acquis.yaml
source: http
listen_addr: 0.0.0.0:8081
path: /traefik
auth_type: basic_auth
basic_auth:
  username: traefik
  password: change-me
labels:
  type: traefik
```

Authentication is mandatory on CrowdSec's side. `basic_auth` needs nothing here
beyond the userinfo in `ACCESSLOGEXPORTER_URL`, as above; for
`auth_type: headers` use `ACCESSLOGEXPORTER_HEADERS: "x-api-key: change-me"` and
drop it from the URL. The `http` source needs a port of its own — `8080` is
CrowdSec's own LAPI.

#### Multiple headers

`ACCESSLOGEXPORTER_HEADERS` is one `Key: Value` per line, so several headers mean
a multi-line value — a YAML block scalar in compose and in a manifest:

```yaml
    environment:
      ACCESSLOGEXPORTER_HEADERS: |-
        x-api-key: change-me
        x-tenant: eu-west
```

```yaml
          - name: ACCESSLOGEXPORTER_HEADERS
            value: |-
              x-api-key: change-me
              x-tenant: eu-west
```

From a shell it is
`-e $'ACCESSLOGEXPORTER_HEADERS=x-api-key: change-me\nx-tenant: eu-west'`.
Whitespace around the key and value is trimmed, only the first colon splits the
line so a value may contain more of them, and lines without a colon are ignored.
Repeating a key overwrites it rather than sending the header twice, and a
`Content-Type` here is overwritten by `ACCESSLOGEXPORTER_CONTENT_TYPE`.

The variable is read from the container environment, which every process
inherits. container-supervisor can also scope it to this one process, but only
from a config file mounted over `/container-supervisor/config.yml` — and that
file replaces the one in the image, so it has to declare `traefik` and
`certwatcher` again as well:

```yaml
processes:
  accesslogexporter:
    path: /home/nonroot/access-log-exporter
    type: ticker
    ticker: "@every 5s"
    hide_label: true
    on_failure: continue
    environment:
      ACCESSLOGEXPORTER_HEADERS: |-
        x-api-key: change-me
        x-tenant: eu-west
```

The `SUPERVISOR_PROCESSES__ACCESSLOGEXPORTER__ENVIRONMENT__*` override form does
**not** work for this: container-supervisor lowercases the whole key path, so the
process is handed `accesslogexporter_headers` and nothing reads it. Set
`ACCESSLOGEXPORTER_HEADERS` on the container instead.

On Kubernetes only the URL changes — no volume, no host path, no sidecar:

```yaml
        env:
          - name: SUPERVISOR_PROCESSES__ACCESSLOGEXPORTER__ENABLED
            value: "true"
          - name: ACCESSLOGEXPORTER_URL
            value: http://traefik:change-me@crowdsec.crowdsec.svc:8081/traefik
          - name: TRAEFIK_ACCESSLOG_FILEPATH
            value: /home/nonroot/config/access.log
          - name: TRAEFIK_ACCESSLOG_FORMAT
            value: json
        volumeMounts:
          - name: config
            mountPath: /home/nonroot/config
      volumes:
        - name: config
          emptyDir: {}
```

With `readOnlyRootFilesystem: true` that `emptyDir` is required: Traefik writes
the log and access-log-exporter writes `access.log.offset` next to it.

#### Access logs leave stdout

Traefik only writes the access log to stdout when no `filePath` is set, so
pointing it at a file to ship it takes those lines off the console.
`ACCESSLOGEXPORTER_ECHO: "true"` puts them back — access-log-exporter prints
every line it reads, so `docker logs` and `kubectl logs` still show them. They
arrive up to
one tick late and out of order with Traefik's own output; the process runs with
`hide_label: true`, so they are unprefixed and parse like stock Traefik output.
Traefik's application log (errors, ACME, routing) stays on stdout either way.

#### Failure handling

The offset on disk is the only state and it advances only on a `200`, so the log
file is the buffer: a failed POST is resent on the next tick and a container
restart resumes where it left off. `on_failure: continue` keeps a failing
exporter from taking Traefik down with it.

A `400` is the exception. CrowdSec answers `400` for malformed JSON, and since
the offset never advances past bytes that were not accepted, one bad line would
otherwise be retried every 5 seconds forever and shipping would stop dead. So a
rejected batch is resent one line at a time; the lines that still `400` are
logged and skipped. A whole batch failing that way means the log is not JSON —
almost always `TRAEFIK_ACCESSLOG_FORMAT` left at `common`, which the log line
says outright. `401`, `413`, `5xx`, timeouts and connection refused are treated
as transient and simply retried.

A half-written line at the end of the file is left alone: only bytes up to the
last newline are ever shipped.

#### Rotation

Nothing else rotates this file, so access-log-exporter truncates it once it has
shipped everything and the file is over `ACCESSLOGEXPORTER_MAX_SIZE`. Traefik
opens the access
log in append mode, so the next line it writes lands at the start of the empty
file — no signal or restart needed. Lines written in the instant between the
last read and the truncate are lost; CrowdSec decides over many requests, so
that window does not matter in practice.

Truncation only ever happens after a run shipped everything, so a CrowdSec
outage can never drop unsent lines. `MAX_SIZE=0` truncates on every run, which
keeps the file at 5 seconds of traffic and is the cheapest setting when nothing
persists the log anyway. `MAX_SIZE=-1` disables it, for a mounted volume with
its own rotation.

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
  `docker logs`, Traefik's `ping` and `metrics` endpoints, or the `-debug` image
  below.
- **Log lines are prefixed.** container-supervisor labels each process's output,
  so `docker logs` shows `[traefik    ] …` and `[certwatcher] …` rather than
  stock Traefik format. Anything parsing the logs has to strip the prefix.
  `access-log-exporter` is the exception — it runs with `hide_label: true` so
  echoed access lines stay unprefixed.
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
| `:v3.7.9`, `:latest` | `scratch` | no shell, no package manager |
| `:v3.7.9-debug`, `:latest-debug` | `scratch` | identical, plus Debian's static busybox — `/bin/sh` and its applets for `docker exec` |

The debug image is the same `final` stage with one extra layer, so it runs the
same binaries as the same uid; only reach for it when you need to look inside a
running container, and don't deploy it.

All under `ghcr.io/basecrusher/rootless-containers/traefik`. The version tag is
`TRAEFIK_VERSION` from `docker-bake.hcl`, which is also the release that gets
downloaded, so the two can't drift; `latest` follows `main`.

The workflow lints the Dockerfile with droast before building and scans the
pushed image with Trivy afterwards, failing on any fixable `HIGH` or `CRITICAL`
vulnerability. A nightly `trivy-traefik` workflow rescans `latest` and
`latest-debug` without rebuilding.

## Cross-platform builds

```sh
cd traefik && docker buildx bake
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `traefik` | `${REGISTRY}/traefik:${TRAEFIK_VERSION}`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `traefik-debug` | `${REGISTRY}/traefik:${TRAEFIK_VERSION}-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY` and `TRAEFIK_VERSION` are bake variables — override either from the
environment (`TRAEFIK_VERSION=v3.7.8 docker buildx bake …`).

The upstream release tarball is named
`traefik_<version>_linux_<arch>.tar.gz`, where `<arch>` is
`$TARGETARCH$TARGETVARIANT` — `amd64`, `arm64`, `armv7`. container-supervisor
publishes its assets under the same suffix. Neither is emulated: the download
stage runs on `$BUILDPLATFORM` and only unpacks.

`certwatcher` and `access-log-exporter` are the two things built rather than
downloaded. Their stages are pinned to `$BUILDPLATFORM` too and Go is pointed at
`$TARGETOS`/`$TARGETARCH`/`$TARGETVARIANT`, so they cross-compile without
emulation as well. Adding a platform to `docker-bake.hcl` is enough as long as
Traefik and container-supervisor both publish a binary for it.
