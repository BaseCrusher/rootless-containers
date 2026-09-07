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
| Configuration | ~60 env vars read by that script | mounted `config.yaml`, `acquis.d`, `cscli` |
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
drove from env vars, a mounted `config.yaml` does — see
[Configuration](#configuration).

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| `crowdsec`, `cscli` | [crowdsecurity/crowdsec](https://github.com/crowdsecurity/crowdsec) | `CROWDSEC_VERSION` — copied out of `crowdsecurity/crowdsec:$CROWDSEC_VERSION` |
| container-supervisor | [container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` |
| envelope | [envelope](https://github.com/BaseCrusher/envelope) | `ENVELOPE_VERSION` — renders `config.yaml.local` from env, [off by default](#generating-configyamllocal-from-env-vars) |

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
datafiles at every start by pointing the baked `install_collections` slot at
`hub upgrade`; it already runs before CrowdSec loads its parsers:

```yaml
        env:
          - name: SUPERVISOR_PROCESSES__INSTALL_COLLECTIONS__ARGUMENTS
            value: hub upgrade
```

That one variable is enough — the slot and CrowdSec's wait on it are already in
the shipped `supervisor.yml`; see [Install collections at
start](#install-collections-at-start).

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

### Acquisition files from env vars

Instead of mounting each `acquis.d/*.yaml`, you can render one from env vars with
[`envelope`](#generating-configyamllocal-from-env-vars) — the same tool the
`config` process uses. There is no baked slot for it (acquisition is one file per
source, and only you know how many), so declare an `envelope` process per source
and point its `-out` at that source's file:

```yaml
    environment:
      SUPERVISOR_PROCESSES__ACQUIS_TRAEFIK__PATH: /usr/local/bin/envelope
      SUPERVISOR_PROCESSES__ACQUIS_TRAEFIK__TYPE: one_shot
      SUPERVISOR_PROCESSES__ACQUIS_TRAEFIK__ARGUMENTS: -prefix ACQUISITION_TRAEFIK_ -out /etc/crowdsec/acquis.d/traefik.yaml
      SUPERVISOR_PROCESSES__CROWDSEC__DEPENDS_ON__ACQUIS_TRAEFIK__EXIT: any

      ACQUISITION_TRAEFIK_source: http
      ACQUISITION_TRAEFIK_listen_addr: 0.0.0.0:8081
      ACQUISITION_TRAEFIK_path: /traefik
      ACQUISITION_TRAEFIK_auth_type: headers
      ACQUISITION_TRAEFIK_headers__X-Api-Token: change-me
      ACQUISITION_TRAEFIK_labels__type: traefik
```

writes `/etc/crowdsec/acquis.d/traefik.yaml`:

```yaml
source: http
listen_addr: 0.0.0.0:8081
path: /traefik
auth_type: headers
headers:
  X-Api-Token: change-me
labels:
  type: traefik
```

The prefix pattern is `ACQUISITION_<NAME>_`; envelope strips it and takes the
rest verbatim (`__` nests, a single `_` is literal, so `listen_addr` and
`headers__X-Api-Token` land as written). Add a second source by declaring
a second process — its own name, prefix and `-out`
(`ACQUIS_SSHD` / `ACQUISITION_SSHD_` / `acquis.d/sshd.yaml`) — and repeat for as
many as you need. Each `DEPENDS_ON` line makes CrowdSec wait for that file before
it reads `acquis.d/`; `exit: any` mirrors the other start-time processes so a
disabled render never blocks startup.

Header names keep their hyphens: envelope copies the key after the prefix
untouched, and Docker and recent Kubernetes pass a hyphenated env-var name
through (a POSIX shell's `export` will not — set it via `-e`/`environment`/the
pod `env:` list). A token like `X-Api-Token`'s value is a secret; keep it out of
the env block and read it from a mounted file instead — see
[Loading secrets from files](#loading-secrets-from-files-_file).

## Loading secrets from files (`*_FILE`)

`cscli`, `crowdsec` and `envelope` all read plain env vars, so a secret set that
way — a bouncer key, an acquisition token, a database password — sits in the
container's environment for anything to read. The `load_secrets` process is the
Docker `*_FILE` convention: for every `NAME_FILE` variable pointing at a path it
reads the file and hands the later processes `$NAME` directly, so the value comes
from a mounted Secret and never from the env block.

It runs **first on every start and is enabled by default**; with no `_FILE`
variables set it does nothing. To use it, mount the secret and point a `_FILE`
twin of the variable you would otherwise set at it:

```yaml
    environment:
      SUPERVISOR_PROCESSES__CONFIG__ENABLED: "true"
      CROWDSEC_CONFIG_db_config__password_FILE: /run/secrets/db-password
      ACQUISITION_TRAEFIK_headers__X-Api-Token_FILE: /run/secrets/traefik-token
    secrets:
      - db-password
      - traefik-token
```

`load_secrets` materialises `CROWDSEC_CONFIG_db_config__password` and
`ACQUISITION_TRAEFIK_headers__X-Api-Token` from the mounted files before the
`config` and acquisition renders read them, so the `.local` overlay and the
acquisition file get the secret without it ever being an env var.

The `_FILE` name is the full variable plus `_FILE`
(`CROWDSEC_CONFIG_db_config__password` → `…password_FILE`), and a `_FILE` twin
takes precedence — set one or the other, not both. `load_secrets` runs at the
supervisor's default `on_failure: fail`, so if it cannot read the file a `_FILE`
points at, the container aborts rather than starting without the secret.

Every **baked** start-time process (`register`, `install_collections`, `config`,
`crowdsec`) already waits for `load_secrets`. An acquisition or bootstrap process
you [define yourself](#acquisition-files-from-env-vars) that consumes a secret
must add the same wait so the file exists before it reads env:

```yaml
      SUPERVISOR_PROCESSES__ACQUIS_TRAEFIK__DEPENDS_ON__LOAD_SECRETS__EXIT: any
```

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

### Install collections at start

`supervisor.yml` ships an `install_collections` process — a `one_shot` `cscli`
step that runs before CrowdSec, so a collection is in place for the same start
rather than the next one. It carries no arguments, so by default it runs `cscli`
with none (prints help, exits 0, a harmless no-op). Give it the install command
through one env var:

```yaml
services:
  crowdsec:
    image: ghcr.io/basecrusher/rootless-containers/crowdsec:v1.7.8-1.0
    environment:
      SUPERVISOR_PROCESSES__INSTALL_COLLECTIONS__ARGUMENTS: collections install crowdsecurity/traefik
    volumes:
      - csdata:/var/lib/crowdsec/data
      - csconfig:/etc/crowdsec
```

That is the whole change — the dependency wiring is already baked in: `crowdsec`
`depends_on` `install_collections` `exit: any`, so CrowdSec waits for it and the
collection loads on the same start rather than the next boot. The step has no
`depends_on` of its own: installing a collection is a hub-and-filesystem
operation that never touches the LAPI or the database, so it does not wait on
`register` and still works when `register` is disabled (remote LAPI).

`cscli collections install` is idempotent: an already-installed item is skipped
(exit 0), so keep the full list of collections you want in the variable and let
it re-assert them every start — it installs the missing ones and no-ops the rest.
Add a collection by appending its name; `install` takes several at once:

```yaml
      SUPERVISOR_PROCESSES__INSTALL_COLLECTIONS__ARGUMENTS: collections install crowdsecurity/traefik crowdsecurity/sshd
```

Two things this needs. **Network** — the baked hub index carries no item
content, so the first install of each item downloads it. And **the
`/etc/crowdsec` volume** (`csconfig` above): installed items are symlinks under
`/etc/crowdsec/collections` into `/etc/crowdsec/hub`, owned by uid 65532 like the
process, so the install writes fine — but without a volume there they are gone on
recreation and re-downloaded every start. `on_failure: continue` is baked in, so
even a genuine failure (no network on a fresh volume) logs and lets CrowdSec come
up rather than aborting.

Arguments are split on whitespace, so quoting inside the value does nothing; one
word per argument. For an argument that must contain a space, number the entries
instead (`SUPERVISOR_PROCESSES__INSTALL_COLLECTIONS__ARGUMENTS__0=…`, `__1=…`).

### Register bouncers at start

A bouncer's API key is a database row, so it is seeded at start rather than
exec'd in afterwards. `supervisor.yml` ships an `add_bouncer` process that uses
container-supervisor's [`for_each`](https://github.com/BaseCrusher/container-supervisor)
to register **one bouncer per row** from a single definition:

```yaml
  add_bouncer:
    path: /usr/local/bin/cscli
    type: one_shot
    on_failure: continue
    arguments: ["bouncers", "add", "{{1}}", "-k", "$(BOUNCER_KEY_{{2}})"]
    for_each:
      - "traefik;TRAEFIK"
    depends_on:
      load_secrets:
        exit: any
      register:
        exit: success
```

`crowdsec` `depends_on add_bouncer exit: any`, so the agent waits for it and a
failed or skipped registration never blocks startup. To use it, mount the key and
point its `_FILE` twin at the mount:

```yaml
    environment:
      BOUNCER_KEY_TRAEFIK_FILE: /run/secrets/CROWDSEC_BOUNCER_KEY
    secrets:
      - CROWDSEC_BOUNCER_KEY
```

How the pieces fit:

- **`for_each`** turns one process into one instance per row. Each row is
  `;`-separated fields; `{{1}}`, `{{2}}` in `arguments` are replaced by that row's
  fields, so `traefik;TRAEFIK` runs `cscli bouncers add traefik -k
  $(BOUNCER_KEY_TRAEFIK)`. The two fields are the **bouncer name** (as CrowdSec
  stores it) and the **key-variable suffix** (uppercase, matching the secret) —
  keeping them separate lets the name stay lowercase while the env var follows the
  `BOUNCER_KEY_*` convention. An empty `for_each` is a fatal supervisor error, so
  the shipped list carries the `traefik` default rather than nothing.
- **`$(BOUNCER_KEY_TRAEFIK)`** is expanded from the environment at launch. The key
  never appears in the manifest: `BOUNCER_KEY_TRAEFIK_FILE` points
  [`load_secrets`](#loading-secrets-from-files-_file) at a mounted Secret, it
  writes `BOUNCER_KEY_TRAEFIK` into the supervisor's env, and `add_bouncer` — which
  `depends_on load_secrets` — reads it there. So the key is a property of a
  Kubernetes Secret or Swarm config, not something you `docker exec` once.
- **`depends_on register: success`** because `bouncers add` writes the LAPI
  database that `register` creates. In the [remote-LAPI](#an-agent-without-a-local-api)
  setup where `register` is disabled, `add_bouncer` is skipped — add bouncers on
  the remote LAPI there.
- **`on_failure: continue`** makes re-adding an already-registered bouncer
  harmless: the second start's `bouncers add` fails, the row keeps the key you
  first passed, and startup proceeds. It also absorbs the default `traefik` row
  when no `BOUNCER_KEY_TRAEFIK` is set.

Register **more or different bouncers** by overriding the row list from env —
each `FOR_EACH__N` is one bouncer, with its own `BOUNCER_KEY_<SUFFIX>_FILE`:

```yaml
    environment:
      SUPERVISOR_PROCESSES__ADD_BOUNCER__FOR_EACH__0: traefik;TRAEFIK
      SUPERVISOR_PROCESSES__ADD_BOUNCER__FOR_EACH__1: metrics;METRICS
      BOUNCER_KEY_TRAEFIK_FILE: /run/secrets/traefik-bouncer-key
      BOUNCER_KEY_METRICS_FILE: /run/secrets/metrics-bouncer-key
    secrets:
      - traefik-bouncer-key
      - metrics-bouncer-key
```

`crowdsec` waits for **every** expanded instance. Bootstrapping the database this
way runs on every replica, so on a [shared database](#multiple-instances-on-a-shared-database)
enable it on one instance only.

### Daily collection upgrades

`install` pins the version it first fetched; it does not pull newer ones. To keep
them current, `supervisor.yml` ships an `upgrade_collections` process — a `cron`
job that runs `cscli collections upgrade --all` daily at 03:00, bumping every
installed collection (and the parsers and scenarios they pull in) without a
container restart. `on_failure: continue`, so a failed run (usually no network)
is logged and the next day tries again rather than aborting the container.

The time is container-local; distroless defaults to UTC, so set `TZ` to move it.
Override the schedule from env vars — `SUPERVISOR_PROCESSES__UPGRADE_COLLECTIONS__CRON`
takes any 5-field cron expression — or disable it by pointing the process at
`/bin/true`. Upgrading needs network; the baked hub index carries no item content.

### More than one bootstrap command

Env vars can also **define a new process from scratch**, not only override the
baked one — set `SUPERVISOR_PROCESSES__<NAME>__PATH`/`__TYPE`/`__ARGUMENTS` and
wire CrowdSec to wait with `SUPERVISOR_PROCESSES__CROWDSEC__DEPENDS_ON__<NAME>__EXIT: any`
(it **merges** into `crowdsec`'s existing `depends_on`). But past one extra
command that gets unwieldy — mount your own supervisor configuration over
`/container-supervisor/config.yml`. It replaces the file in the image, so it has
to declare `register` and `crowdsec` again as well — and the baked
`install_collections`, `config` and `upgrade_collections` are gone unless you
re-declare them too:

```yaml
hide_labels: true

processes:
  register:
    path: /usr/local/bin/cscli
    arguments: ["machines", "add", "localhost", "--auto", "--force"]
    type: one_shot
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
running it at every start (via `install_collections` or a start-time process)
leaves a trail of dead ones.

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

### Overriding settings with a mounted file

Nothing here rewrites `config.yaml` at start, so to change a setting you mount
your own file over it — a Kubernetes ConfigMap, a Swarm config, or a bind mount.
CrowdSec reads two files: `config.yaml`, and `config.yaml.local` — an
*overwrite* file whose values take precedence. Mount over either:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: crowdsec-config
data:
  config.yaml.local: |
    api:
      server:
        listen_uri: 0.0.0.0:9999
    common:
      log_level: debug
    db_config:
      use_wal: true
---
# in the pod spec
volumeMounts:
  - name: crowdsec-config
    mountPath: /etc/crowdsec/config.yaml.local
    subPath: config.yaml.local
    readOnly: true
```

and CrowdSec says so on startup:

```
level=info msg="Loading yaml file: '/etc/crowdsec/config.yaml' with additional values from '/etc/crowdsec/config.yaml.local'"
```

- Use `config.yaml.local` to override a handful of keys and keep the baked
  defaults for the rest; mount over `config.yaml` to replace the file wholesale.
- Overrides in `.local` **merge** into mappings but **replace** sequences whole,
  and a key cannot be *removed* this way, only set to another value — that is
  CrowdSec's
  [`.local` mechanism](https://docs.crowdsec.net/docs/configuration/crowdsec_configuration/).
- Use a `subPath` mount (as above) so only the one file is replaced and the rest
  of `/etc/crowdsec` — the baked hub symlinks and credentials — stays intact.

### Generating `config.yaml.local` from env vars

When a mounted file is awkward — you want a few overrides driven by the same env
the rest of the deployment uses — `supervisor.yml` ships a `config` process that
runs [`envelope`](https://github.com/BaseCrusher/envelope) to build
`config.yaml.local` from `CROWDSEC_CONFIG_`-prefixed variables before CrowdSec
starts. It is **disabled by default**; enable it and set the values:

```yaml
    environment:
      SUPERVISOR_PROCESSES__CONFIG__ENABLED: "true"
      CROWDSEC_CONFIG_common__log_level: debug
      CROWDSEC_CONFIG_api__server__listen_uri: 0.0.0.0:9999
      CROWDSEC_CONFIG_db_config__use_wal: "true"
```

writes `/etc/crowdsec/config.yaml.local`:

```yaml
common:
  log_level: debug
api:
  server:
    listen_uri: 0.0.0.0:9999
db_config:
  use_wal: true
```

envelope's rules: `__` becomes nesting, a numeric segment becomes a list index
(`…__ARGUMENTS__0`), and the key is taken **verbatim** — it is not lowercased, so
write the CrowdSec keys in their own case (`common`, `db_config`), lowercase,
under the uppercase `CROWDSEC_CONFIG_` prefix. It writes the file directly (`-out`),
no shell needed, and `crowdsec` `depends_on` it `exit: any`, so when enabled
CrowdSec waits for the file and when disabled it is skipped and startup proceeds.

This is a `.local` overlay like the mounted file above, so the same merge rules
apply. It writes into `/etc/crowdsec`, which must be writable by uid 65532 (the
baked default; a read-only mount there fails the write and, as a `one_shot` left
at `on_failure: fail`, aborts the start rather than running against a stale one).
Mount your own `config.yaml.local` instead if you would rather not template.

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

### Multiple instances on a shared database

`register` runs `cscli machines add localhost --auto --force`, and `localhost`
is a fixed name. That is fine for one container: `--force` just rewrites its own
row every start. Scale to several instances against **one shared database**
(Postgres or MySQL via `config.yaml`'s `db_config`, every instance reaching it
directly) and they all register the *same* name — each start rewrites that one
row's credentials, the last to boot wins, and every other agent is left holding
stale credentials that no longer authenticate.

`cscli machines add` writes straight to the database, so the fix is only to give
each instance a **unique** name — no image change:

```yaml
        env:
          - name: SUPERVISOR_PROCESSES__REGISTER__ARGUMENTS
            value: machines add $(POD_NAME) --auto --force
```

The override is a literal argument list, not a shell — it does not expand a
variable itself. Resolve the name where the orchestrator can (Kubernetes
downward API into `POD_NAME`, a Compose replica index, the container name) and
pass the resolved value in. Each instance then owns its own row, and `--force`
only ever rewrites that one.

Two things that shared database changes, unrelated to the name:

- **First-boot schema race.** On a brand-new empty database, N instances all
  running `machines add` at once each try to create the schema. Bring up one
  instance first (or run `cscli` against the database once), then scale out.
  Concurrent registers against an existing schema are fine.
- **Bootstrap once, not per replica.** A start-time `cscli` step that writes the
  database — the [`add_bouncer`](#register-bouncers-at-start) process, not the
  hub-only `install_collections` — runs on every instance and collides exactly the
  way `localhost` did. Enable it on a single init instance, or give each a unique
  bouncer name.

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
| `:v1.7.8-1.0`, `:v1.7.8-1`, `:latest` | `static-debian13:nonroot` | no shell, no package manager |
| `:v1.7.8-1.0-debug`, `:v1.7.8-1-debug`, `:latest-debug` | `static-debian13:debug-nonroot` | identical, plus busybox at `/busybox/sh` |

All under `ghcr.io/basecrusher/rootless-containers/crowdsec`. Tags are
`<version>-Y.Z`: `<version>` is `CROWDSEC_VERSION` (the upstream image the
binaries are copied from, so the tag and what is inside cannot drift), `Y.Z` is
`IMAGE_REVISION` — `Y` for breaking repackaging of the same CrowdSec, `Z` for
fixes that don't. `<version>-Y` is a rolling tag that always points at the
newest `Z` of that revision. The workflow aborts before building any tag that
isn't `<version>-Y` or `<version>-Y.Z` (see the repo README). `latest` follows
`main`.

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
| `crowdsec` | `${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${IMAGE_REVISION}`, `:${CROWDSEC_VERSION}-<Y>`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `crowdsec-debug` | `${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${IMAGE_REVISION}-debug`, `:${CROWDSEC_VERSION}-<Y>-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY`, `CROWDSEC_VERSION` and `IMAGE_REVISION` (default `1.2`) are bake
variables — override any from the environment
(`CROWDSEC_VERSION=v1.7.7 docker buildx bake …`). Nothing is
compiled and nothing is emulated: the binaries come from the upstream image for
the target platform, and the stage that edits the configuration and downloads
container-supervisor runs on `$BUILDPLATFORM`. Adding a platform works as long
as `crowdsecurity/crowdsec` and container-supervisor both publish one.
