# crowdsec — build internals

Nothing is compiled here. `crowdsec` and `cscli` are copied out of
`crowdsecurity/crowdsec:$CROWDSEC_VERSION`, which builds them with
`BUILD_STATIC=1` — fully static ELF binaries, which is the only reason musl
binaries from an Alpine image run on a Debian-based distroless base. If upstream
ever drops that flag the image still builds and fails at *runtime*, so a build
that succeeds is not evidence: run the image.

## Two stages from the same image

`upstream` has no platform pin, so buildx pulls the target platform's variant and
the `COPY` takes its binaries. `config` is pinned to `$BUILDPLATFORM` because it
runs `yq` and `wget`, and emulating that would be pointless: everything it
touches — the hub YAML, the GeoLite2 databases, `config.yaml` — is
architecture-independent. `wget` and `yq` are both already in the upstream image,
so no third stage is needed to download container-supervisor and `envelope` —
both fetched for `$TARGETARCH$TARGETVARIANT` (their linux arm/v7 asset is named
`…-armv7`, so the same suffix that works for the supervisor works for envelope),
not for the build platform.

## The paths cannot move

Every installed hub item in `/etc/crowdsec` is an **absolute** symlink into
`/etc/crowdsec/hub/`:

```
/etc/crowdsec/collections/sshd.yaml -> /etc/crowdsec/hub/collections/crowdsecurity/sshd.yaml
```

So the usual `/home/nonroot` layout of the other images is not an option — moving
the tree turns every collection, parser and scenario into a dangling symlink, and
CrowdSec starts with nothing loaded rather than failing. `/etc/crowdsec` and
`/var/lib/crowdsec` therefore stay where upstream puts them and are `COPY
--chown=65532:65532`'d instead, which also keeps upstream's documentation and
volume paths valid.

`/container-supervisor/supervisor_environment` — the supervisor's `env_dir`, where
`load_secrets` writes materialised secrets — is `COPY --chown=65532:65532`'d as an
empty directory (created with `mkdir` in the `config` stage) for the same reason:
`/container-supervisor` is root-owned (the `config.yml` copy makes it so), and the
process writes there as uid 65532 on every start.

The content is copied from `/staging/etc/crowdsec` and
`/staging/var/lib/crowdsec` in the upstream image, but the `/staging` mechanism
itself is dropped: it exists so `docker_start.sh` can `rsync` the pristine
configuration into a mounted-and-empty `/etc/crowdsec`, and there is no shell
here to do that. The consequence is documented in the README — an empty
directory mounted over `/var/lib/crowdsec/data` hides the preloaded datafiles,
where the official image would relink them from `/staging` on every start.

## config.yaml edits

The `yq` expression in the `config` stage does what upstream's `docker_start.sh`
does at runtime, minus everything driven by env vars:

- `del(.plugin_config)` — it is `user: nobody`, and setuid to a *different* user
  needs root. Nothing reads it while the notification plugins are absent.
- `del(.config_paths.plugin_dir)` — the plugins are not shipped: 94 MB, and inert
  until a profile references one.

Shipping the plugins is a supported combination, tested, and it is not root that
stands in the way — `notification-file` launches and delivers as uid 65532. It
needs all three of: the plugins `COPY --chown=65532:65532`'d (CrowdSec refuses
`plugin at … is not owned by user 'nonroot'`), `plugin_dir` kept, and
`plugin_config: {user: nonroot, group: nonroot}` — naming the user the process
already is, which needs no privilege, unlike upstream's `nobody`. Miss the
`plugin_config` and a profile that names a notification is fatal at startup
(`plugins are enabled, but the plugin_config section is missing`), so the three
edits move together or not at all.
- `del(.api.server.tls)` — upstream ships `agents_allowed_ou`/
  `bouncers_allowed_ou` with no certificate paths and deletes the whole key
  unless `USE_TLS` is set.
- `common.log_dir` — `/var/log` does not exist on distroless. `log_media` is
  `stdout` so nothing writes there today; this only matters if someone sets
  `log_media: file`.
- `api.server.online_client.credentials_path` — pointed at the empty
  `online_api_credentials.yaml` that upstream already ships. CrowdSec warns and
  disables CAPI while the file has no `login`, and picks it up once
  `cscli capi register -f` fills it in, so enabling CAPI needs no config edit.

## Startup: register, then CrowdSec

`container-supervisor` is the entrypoint, with `supervisor.yml` baked in at its
default config path. Runtime configuration is a file on disk
(`config.yaml`/`config.yaml.local`) that CrowdSec reads directly; the `config`
process only *writes* the `.local` overlay from env and is off by default, so by
default nothing here templates `config.yaml` — it is used as mounted.

- `load_secrets` — `prepare_secrets`, the built-in supervisor type, first and
  **disabled by default** — the same opt-in-helper shape as coredns' `a-records`.
  When enabled it reads the `*_FILE` variables named in its `secrets:` list, and for
  each `NAME_FILE` pointing at a path writes `env_dir/NAME`, which the supervisor
  auto-loads into every later process's environment — the Docker `*_FILE` convention
  that `cscli`, `crowdsec` and `envelope` do not implement themselves. It is off by
  default because the secret set is operator-defined at runtime (arbitrary bouncer
  keys, db passwords, acquisition tokens) and cannot be enumerated at build time:
  the common deployment uses no `_FILE` secret and should run no no-op process. An
  operator opts in by flipping `SUPERVISOR_PROCESSES__LOAD_SECRETS__ENABLED=true`,
  mounting each `_FILE` var, and naming it in the list, e.g.
  `SUPERVISOR_PROCESSES__LOAD_SECRETS__SECRETS__0=BOUNCER_KEY_TRAEFIK_FILE`. The
  explicit list is the 1.9.2 model — before it, `prepare_secrets` auto-scanned every
  `*_FILE` and needed no list; 1.9.2..1.10.0 then rejected an empty list as a fatal
  startup error, so **1.10.1** is the floor here because it made an empty/omitted
  list a clean no-op again (enabling `load_secrets` without a list must not crash).
  A *listed* var that is unset, or whose file cannot be read, fails the process.
  Every start-time process and `crowdsec` `depends_on` it
  `exit: success, ignore_exit_when_disabled: true` (container-supervisor 1.10.0+) —
  the same edge coredns' `corefile-gen` puts on `a-records`: the barrier orders it
  first (so the file is on disk before a consumer reads env), `ignore_exit_when_disabled`
  lets the default-disabled `load_secrets` pass the gate rather than skip every
  dependent, and `success` still requires it to succeed *when enabled*. It keeps the
  default `on_failure: fail`, so if an enabled `load_secrets` cannot read a file a
  `_FILE` points at, that aborts the container loudly rather than starting without
  the credential. `env_dir` defaults
  to `/container-supervisor/supervisor_environment` (beside the config); that
  directory is pre-created `--chown=65532:65532` in the Dockerfile because
  `/container-supervisor` itself is root-owned and the process runs every start.
- `register` — `one_shot`, `cscli machines add localhost --auto --force`. This is
  what upstream's script guards with a "already registered?" check; `--force`
  makes the guard unnecessary, and re-registering at every start is harmless
  because it rewrites both the row and the credentials file. Creating the sqlite
  database is a side effect of it.
- `install_collections` — `one_shot`, `on_failure: continue`, **no**
  `depends_on`. It carries no `arguments`, so with nothing set it runs `cscli`
  bare (prints help, exits 0, a no-op). The operator supplies the install command
  through one env var, `SUPERVISOR_PROCESSES__INSTALL_COLLECTIONS__ARGUMENTS`.
  Baking the slot (rather than making the operator define a process from scratch
  and re-wire CrowdSec) turns bootstrapping a collection into setting a single
  variable. It does **not** `depends_on` `register`: installing a hub item is a
  download-and-symlink into `/etc/crowdsec`, never an LAPI or database call, so
  coupling it to `register` would only break it in the remote-LAPI setup where
  `register` is disabled (a disabled dependency counts as failure and would skip
  the install).
- `add_bouncer` — `one_shot`, `on_failure: continue`, a `for_each` process that
  runs `cscli bouncers add {{1}} -k $(BOUNCER_KEY_{{2}})` once per row. Each
  `;`-field row is `<bouncer-name>;<KEY_SUFFIX>`; the key is expanded at launch
  from `BOUNCER_KEY_<SUFFIX>`, which an enabled `load_secrets` materialises from a
  mounted `_FILE` Secret, so it is never in the manifest. Unlike `install_collections` it
  **does** `depends_on register: success` — `bouncers add` writes the LAPI
  database `register` creates — so it is skipped in the remote-LAPI setup where
  `register` is off, which is correct (bouncers belong on the remote LAPI there).
  `for_each` **cannot be empty** (fatal `for_each is set but empty`), so the file
  ships a single `traefik;TRAEFIK` row as the default; the operator overrides the
  list with `SUPERVISOR_PROCESSES__ADD_BOUNCER__FOR_EACH__N` and supplies each
  key. `on_failure: continue` absorbs the default row when no key is set and the
  re-add failure on every later start.
- `config` — `one_shot`, `enabled: false`. `envelope -prefix CROWDSEC_CONFIG_
  -out /etc/crowdsec/config.yaml.local` renders the `.local` overlay from
  `CROWDSEC_CONFIG_`-prefixed env before CrowdSec reads it. envelope writes to
  **stdout** by default; the `-out` flag (its README omits it, `cmd/envelope`
  has it) makes it write the file, which is why no shell is needed for the `>`
  redirect the examples show. Off by default because most deployments mount their
  own `config.yaml.local`; the operator flips
  `SUPERVISOR_PROCESSES__CONFIG__ENABLED=true`. Left at `on_failure: fail` (like
  coredns' `corefile-gen`): if the override can't be written, abort rather than
  start against a stale one.
- `crowdsec` — `service`, `depends_on` `register: success` **and**
  `install_collections: any` **and** `add_bouncer: any` **and** `config: any`, so
  the collection loads, the bouncers register and the overlay is written on the
  same start rather than the next boot. `any` on
  `install_collections`/`add_bouncer`/`config` because a failed or no-op step, or
  a disabled one, must not block CrowdSec — a disabled dependency counts as
  failure, so `success` there would skip CrowdSec whenever that step stays off.
  A `for_each` dependency expands to every instance, so this one edge makes
  CrowdSec wait for all `add_bouncer` rows. These edges, not a `register`
  dependency, are what order them before CrowdSec.
- `upgrade_collections` — `cron` `0 3 * * *`, `cscli collections upgrade --all`,
  `on_failure: continue`. Keeps installed collections current without a restart.
  No `depends_on`: it fires on wall-clock time, long after startup, and a failed
  run (no network) must not abort the container. Time is container-local — UTC on
  distroless unless `TZ` is set.

`ARGUMENTS` splits on whitespace, so a comma-separated or JSON-looking value
arrives as one argument and `cscli` prints its help; number the entries
(`…__ARGUMENTS__0`, `__1`) for an argument that must contain a space. An operator
that wants a *different* bootstrap command (`bouncers add …`, `hub upgrade`) can
still **define a whole process from env vars** — container-supervisor creates a
process that is not in the file — and merge the wait into `crowdsec`'s
`depends_on` (`SUPERVISOR_PROCESSES__CROWDSEC__DEPENDS_ON__<NAME>__EXIT=any`). The
README documents the full set.

The `depends_on` graph is validated before anything starts — a dangling
reference (`crowdsec` depending on a process that does not exist) is a *fatal
startup error*, not a silent skip, which is why `install_collections` is a real
baked slot and not something the dependency merely hopes will be defined.

`hide_labels: true` drops the `[<process>]` prefix from child output, so
CrowdSec's log lines reach `docker logs` in stock format. The supervisor's own
lines keep the label.

No `setcap` anywhere: 8080 and 6060 are unprivileged.
