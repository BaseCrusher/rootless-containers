# crowdsec

[CrowdSec](https://www.crowdsec.net/) on distroless — the release binaries from
the official image, its preloaded hub and datafiles, and nothing else. No shell,
no package manager, no `docker_start.sh`.

## How it differs from `crowdsecurity/crowdsec`

| | official image | this image |
| --- | --- | --- |
| Base | Alpine | `gcr.io/distroless/static-debian13` |
| User | `root` | uid/gid `65532` |
| PID 1 | `bash /docker_start.sh` | `container-supervisor` |
| Configuration | ~60 env vars read by that script | `CROWDSEC_CONFIG_*` env vars, `acquis.d`, `cscli` |
| Notification plugins | included | not included |
| `source: docker` acquisition | works | not possible — [why](#source-docker-does-not-work-here) |
| Shell, `bash`, `rsync`, `yq` | present | none |

The binaries are the upstream ones, unmodified — they are built static, so they
run on distroless as they are. `/etc/crowdsec` and `/var/lib/crowdsec` are the
stock paths with the stock content, owned by uid 65532.

### The env vars from the official image do not work

`COLLECTIONS`, `BOUNCER_KEY_*`, `DISABLE_ONLINE_API`, `ENROLL_KEY` and the rest
are read by upstream's `docker_start.sh`, which is a 500-line bash script this
image does not ship. What it did at every start, you do once — see
[Bootstrapping with cscli](#bootstrapping-with-cscli).

What upstream's script did unconditionally is already done here: the
configuration directory is populated (baked, not rsynced from `/staging`) and
the local agent is registered at every start by `container-supervisor`. What it
drove from env vars, `CROWDSEC_CONFIG_*` does for `config.yaml` — see
[Configuration](#configuration).

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| `crowdsec`, `cscli` | [crowdsecurity/crowdsec](https://github.com/crowdsecurity/crowdsec) | `CROWDSEC_VERSION` — copied out of `crowdsecurity/crowdsec:$CROWDSEC_VERSION` |
| container-supervisor | [container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` |
| envelope | [envelope](https://github.com/BaseCrusher/envelope) | `ENVELOPE_VERSION` |

Preloaded from the official image: the hub index, the `crowdsecurity/linux`
collection (syslog, sshd, its scenarios), `crowdsecurity/whitelists`,
`crowdsecurity/whitelist-good-actors`, and the datafiles under
`/var/lib/crowdsec/data` — including the ~70 MB of GeoLite2 databases, so geoip
enrichment works offline on first start.

## Usage

```sh
docker run --rm -p 8080:8080 \
  -v csdata:/var/lib/crowdsec/data \
  -v ./acquis.yaml:/etc/crowdsec/acquis.d/traefik.yaml:ro \
  ghcr.io/basecrusher/rootless-containers/crowdsec:v1.7.8-1.0
```

That is a working LAPI on `8080` plus an agent reading whatever `acquis.d`
names. Nothing else has to be configured.

### Persist `/var/lib/crowdsec/data`

The SQLite database lives at `/var/lib/crowdsec/data/crowdsec.db` — decisions,
machine credentials and every bouncer API key. Without a volume there, bouncers
stop authenticating the moment the container is recreated.

Use a **named volume**, not a bind mount or an `emptyDir`. Docker seeds a named
volume from the image, so the preloaded datafiles survive the mount with their
uid 65532 ownership intact. An empty directory mounted over that path hides
them, and CrowdSec then starts without geoip:

```
level=error msg="unable to open GeoLite2-City.mmdb : open /var/lib/crowdsec/data/GeoLite2-City.mmdb: no such file or directory"
```

It is not fatal — parsing and scenarios still work, only the enrichment is
missing. If the path has to be an empty volume (Kubernetes), download the
datafiles at every start with `hub upgrade`, which runs before CrowdSec loads
its parsers:

```yaml
        env:
          - name: SUPERVISOR_PROCESSES__CSCLI__ENABLED
            value: "true"
          - name: SUPERVISOR_PROCESSES__CSCLI__ARGUMENTS
            value: hub upgrade
```

### Acquisition

`/etc/crowdsec/acquis.d/` is where acquisition files go, one per source. The
baked `/etc/crowdsec/acquis.yaml` is upstream's placeholder — a `file` source
pointing at `/does/not/exist`, which lets the container start with no
acquisition at all and logs one warning:

```
level=warning msg="No matching files for pattern /does/not/exist"
```

### `source: docker` does not work here

Reading container logs off the Docker socket is not an option: the socket is
`root:root`, so uid 65532 cannot read it, and the only ways to change that —
`--group-add` its gid, or running as root — hand the container
root-equivalent control of the host, which is the entire thing this image exists
to avoid. Mounting it defeats the image.

Have the log producer push to CrowdSec instead: the
[traefik](../traefik/) image's `access-log-exporter` POSTs Traefik's access log
to an `http` source, which needs no volume and no socket, and works the same on
Swarm and Kubernetes.

```yaml
# acquis.d/traefik.yaml
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

The `http` source needs a port of its own — `8080` is the LAPI, `6060` the
Prometheus endpoint.

## Bootstrapping with cscli

Registering a bouncer, installing a collection, enrolling in the console —
everything upstream's script drove from env vars is a `cscli` call, and `cscli`
is in the image at `/usr/local/bin/cscli`. It works over `docker exec` even
without a shell, because it is the process being executed:

```sh
docker exec crowdsec cscli bouncers add traefik -k "$(openssl rand -hex 16)"
docker exec crowdsec cscli collections install crowdsecurity/traefik
docker exec crowdsec cscli capi register -f /etc/crowdsec/online_api_credentials.yaml
docker exec crowdsec cscli console enroll "$ENROLL_KEY"
```

Anything that ends up in the database (bouncers, decisions, machines) persists
with the data volume. Anything that ends up in `/etc/crowdsec` (installed hub
items, CAPI credentials) does not, unless that path is a volume too — otherwise
it is gone the next time the container is recreated.

### Declaratively, without exec

`container-supervisor` runs one `cscli` command of your choice before CrowdSec
starts. The `cscli` process is in `supervisor.yml` as a disabled placeholder;
enable it and give it arguments:

```yaml
services:
  crowdsec:
    image: ghcr.io/basecrusher/rootless-containers/crowdsec:v1.7.8-1.0
    environment:
      SUPERVISOR_PROCESSES__CSCLI__ENABLED: "true"
      SUPERVISOR_PROCESSES__CSCLI__ARGUMENTS: collections install crowdsecurity/traefik
    volumes:
      - csdata:/var/lib/crowdsec/data
      - csconfig:/etc/crowdsec
```

Arguments are split on whitespace, so quoting inside that value does nothing —
one word per argument. CrowdSec starts only after this exits, whatever the
exit code (`on_failure: continue`), so a hub item installed here is loaded in
the same start rather than the next one. Installing needs network: the baked hub
index carries no item content.

For **more than one** command, mount your own supervisor configuration over
`/container-supervisor/config.yml`. It replaces the file in the image, so it has
to declare `register` and `crowdsec` again as well:

```yaml
hide_labels: true

processes:
  config:
    path: /usr/local/bin/envelope
    arguments: ["-prefix", "CROWDSEC_CONFIG_", "-out", "/etc/crowdsec/config.yaml.local"]
    type: one_shot
  register:
    path: /usr/local/bin/cscli
    arguments: ["machines", "add", "localhost", "--auto", "--force"]
    type: one_shot
    depends_on:
      config:
        exit: success
  collections:
    path: /usr/local/bin/cscli
    arguments: ["collections", "install", "crowdsecurity/traefik"]
    type: one_shot
    on_failure: continue
    depends_on:
      register:
        exit: success
  bouncer:
    path: /usr/local/bin/cscli
    arguments: ["bouncers", "add", "traefik", "-k", "0123456789abcdef0123456789abcdef"]
    type: one_shot
    on_failure: continue
    depends_on:
      collections:
        exit: any
  crowdsec:
    path: /usr/local/bin/crowdsec
    type: service
    depends_on:
      bouncer:
        exit: any
```

`cscli bouncers add` is idempotent enough for this: re-adding an existing
bouncer fails, `on_failure: continue` shrugs it off, and the key in the database
stays the one you passed. That makes the API key a property of the manifest —
a Kubernetes Secret or a Swarm config — rather than something you exec once and
have to remember.

## The Central API is off until you register

`api.server.online_client.credentials_path` is set, but the file it points at is
empty, so the community blocklist and signal sharing start out disabled:

```
level=warning msg="can't load CAPI credentials from '/etc/crowdsec/online_api_credentials.yaml' (missing login field)"
level=warning msg="Communication with CrowdSec Central API disabled from configuration file"
```

`cscli capi register -f /etc/crowdsec/online_api_credentials.yaml` fills it in
and the next start picks it up — nothing else to configure. Do that once, with
`/etc/crowdsec` on a volume: every registration creates a new CAPI account, so
running it at every start (via the `cscli` slot) leaves a trail of dead ones.

Registration talks to `api.crowdsec.net`, which is why it is not the default.

## Configuration

`/etc/crowdsec/config.yaml` is upstream's Docker configuration with these edits
made at build time:

| Edit | Why |
| --- | --- |
| `plugin_config` removed | it is `user: nobody, group: nobody`, and setuid to a *different* user needs root |
| `config_paths.plugin_dir` removed | the notification plugins are not shipped |
| `api.server.tls` removed | upstream ships it with `allowed_ou` but no certificates, which upstream's script deletes unless `USE_TLS` is set |
| `common.log_dir` → `/var/lib/crowdsec/log` | `/var/log` does not exist here and would not be writable |
| `api.server.online_client.credentials_path` set | so registering with CAPI is enough to enable it |

Everything else is stock, including `listen_uri: 0.0.0.0:8080`, sqlite, and the
Prometheus endpoint on `6060`.

### `CROWDSEC_CONFIG_*` env vars

CrowdSec reads no env vars of its own, but it does read
`/etc/crowdsec/config.yaml.local` — an *overwrite* file whose values take
precedence over `config.yaml`. That file is written at every start by
[envelope](https://github.com/BaseCrusher/envelope), which turns the
`CROWDSEC_CONFIG_`-prefixed environment into YAML, so a mounted configuration
file is not needed to change a setting:

```yaml
services:
  crowdsec:
    image: ghcr.io/basecrusher/rootless-containers/crowdsec:v1.7.8-1.0
    environment:
      CROWDSEC_CONFIG_common__log_level: debug
      CROWDSEC_CONFIG_api__server__listen_uri: 0.0.0.0:9999
      CROWDSEC_CONFIG_db_config__use_wal: true
```

becomes

```yaml
# /etc/crowdsec/config.yaml.local
api:
    server:
        listen_uri: 0.0.0.0:9999
common:
    log_level: debug
db_config:
    use_wal: true
```

and CrowdSec says so on startup:

```
level=info msg="Loading yaml file: '/etc/crowdsec/config.yaml' with additional values from '/etc/crowdsec/config.yaml.local'"
```

- **`__` (double underscore) descends a level**, a single `_` is an ordinary
  character — which is why `db_config__type` is `db_config: {type: …}`. A segment
  of digits is a list index (`A__0`, `A__1`); see envelope's
  [naming](https://github.com/BaseCrusher/envelope#naming) for the full grammar,
  including lists of mappings.
- **Keys keep the case you write.** CrowdSec's keys are lowercase, so the part
  after the prefix has to be lowercase too. `CROWDSEC_CONFIG_COMMON__LOG_LEVEL`
  produces a `COMMON` key and CrowdSec refuses to start:
  `field COMMON not found in type csconfig.Config`. The same happens for a
  misspelled key, which makes a typo loud rather than silent.
- **Values are typed** the way YAML types them: `8080` is an int, `true` a bool,
  `"8080"` (quoted inside the value) a string.
- The prefix is `CROWDSEC_CONFIG_`, not `CROWDSEC_`, because Kubernetes injects
  `CROWDSEC_PORT`, `CROWDSEC_SERVICE_HOST` and friends into every pod in the
  namespace as soon as a Service is named `crowdsec` — with the shorter prefix
  those would land in the configuration and be fatal.
- Overrides **merge** into mappings but **replace** sequences whole, and a key
  cannot be *removed* this way, only set to another value — that is CrowdSec's
  [`.local` mechanism](https://docs.crowdsec.net/docs/configuration/crowdsec_configuration/),
  not envelope's doing.

With no such variable set the file is written as `{}`, which changes nothing.
Mounting your own `config.yaml.local` therefore does not work — it is
overwritten, or the start fails if the mount is read-only. Mount over
`config.yaml` instead, or drop the `config` process:
`SUPERVISOR_PROCESSES__CONFIG__ENABLED=false` together with
`SUPERVISOR_PROCESSES__REGISTER__DEPENDS_ON__CONFIG__EXIT=any`.

### Notification plugins are not included

The six `notification-*` plugins are 94 MB of the official image and do nothing
until a profile references one, so they are left out. Alerts leave through the
CrowdSec console or a bouncer instead.

**A profile that names a notification is fatal here**, which is worth knowing
before mounting your own `profiles.yaml`:

```
level=fatal msg="api server init: plugins are enabled, but the plugin_config section is missing in the configuration"
```

Rootlessness is not the reason they are missing — they do run as uid 65532. It
takes three things, all at build time, and upstream's own configuration provides
none of them:

- `COPY --from=upstream --chown=65532:65532 /usr/local/lib/crowdsec/plugins/ /usr/local/lib/crowdsec/plugins/` —
  CrowdSec refuses a plugin it is about to run as a user that does not own it
  (`plugin at … is not owned by user 'nonroot'`).
- `config_paths.plugin_dir` kept.
- `plugin_config: {user: nonroot, group: nonroot}` — the container's *own* user.
  Upstream's `nobody` would need a real setuid, which needs root; naming the user
  it already is does not.

### An agent without a local API

The `register` process runs `cscli machines add localhost --auto --force` at
every start, which owns `/etc/crowdsec/local_api_credentials.yaml`. To point the
agent at a remote LAPI instead, disable it and drop CrowdSec's dependency on it:

```yaml
    environment:
      SUPERVISOR_PROCESSES__REGISTER__ENABLED: "false"
      SUPERVISOR_PROCESSES__CROWDSEC__DEPENDS_ON__REGISTER__EXIT: any
    volumes:
      - ./local_api_credentials.yaml:/etc/crowdsec/local_api_credentials.yaml:ro
```

Both are needed: a disabled process counts as a failure to anything that
`depends_on` it, so without the second variable CrowdSec is skipped and the
container exits. Disable the local API itself in `config.yaml`
(`api.server.enable: false`).

## Distroless caveats

- **No shell.** `docker exec` works for the binaries in the image (`cscli`,
  `crowdsec`), not for shell-form commands or health checks. Use `cscli lapi
  status`, the `/health` endpoint, or the `-debug` image below.
- **Log lines are unprefixed.** `supervisor.yml` sets `hide_labels: true`, so
  CrowdSec's output reaches `docker logs` in stock format and anything parsing
  it does not have to know a supervisor is running. The supervisor's own
  `[supervisor]` lines still carry the label.
- **No timezone database.** Go falls back to UTC regardless of `TZ`.
- `/etc/ssl/certs/ca-certificates.crt` is present, so CAPI, hub downloads and
  console enrollment verify normally.

### Ports

- `8080/tcp` — local API
- `6060/tcp` — Prometheus metrics

Both are unprivileged, so no capability is needed on the binary. Any `http`
acquisition source listens on a port you choose.

## Images

| Tag | Base | Notes |
| --- | --- | --- |
| `:v1.7.8-1.0`, `:latest` | `static-debian13:nonroot` | no shell, no package manager |
| `:v1.7.8-1.0-debug`, `:latest-debug` | `static-debian13:debug-nonroot` | identical, plus busybox at `/busybox/sh` |

All under `ghcr.io/basecrusher/rootless-containers/crowdsec`. Tags are
`<version>-Y.Z`: `<version>` is `CROWDSEC_VERSION` (the upstream image the
binaries are copied from, so the tag and what is inside cannot drift), `Y.Z` is
`IMAGE_REVISION` — `Y` for breaking repackaging of the same CrowdSec, `Z` for
fixes that don't. The workflow aborts before building any tag that isn't
`<version>-Y.Z` (see the repo README). `latest` follows `main`.

The workflow lints the Dockerfile with droast before building and scans the
pushed image with Trivy afterwards, failing on any fixable `HIGH` or `CRITICAL`
vulnerability. A nightly `crowdsec-scan` workflow rescans `latest` and
`latest-debug` without rebuilding.

## Cross-platform builds

```sh
cd crowdsec && docker buildx bake
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `crowdsec` | `${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${IMAGE_REVISION}`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `crowdsec-debug` | `${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${IMAGE_REVISION}-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY`, `CROWDSEC_VERSION` and `IMAGE_REVISION` (default `1.0`) are bake
variables — override any from the environment
(`CROWDSEC_VERSION=v1.7.7 docker buildx bake …`). Nothing is
compiled and nothing is emulated: the binaries come from the upstream image for
the target platform, and the stage that edits the configuration and downloads
container-supervisor runs on `$BUILDPLATFORM`. Adding a platform works as long
as `crowdsecurity/crowdsec` and container-supervisor both publish one.
