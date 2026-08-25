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
so no third stage is needed to download container-supervisor — fetched for
`$TARGETARCH$TARGETVARIANT`, not for the build platform.

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
default config path. Runtime configuration is a mounted file
(`config.yaml`/`config.yaml.local`), so there is no `config` process — CrowdSec
reads whatever is on disk directly.

- `register` — `one_shot`, `cscli machines add localhost --auto --force`. This is
  what upstream's script guards with a "already registered?" check; `--force`
  makes the guard unnecessary, and re-registering at every start is harmless
  because it rewrites both the row and the credentials file. Creating the sqlite
  database is a side effect of it.
- `crowdsec` — `service`, `depends_on` `register: success` and nothing else.

There is no baked `cscli` bootstrap slot: it was a disabled placeholder and is
dropped. An operator that wants one bootstrap command (`collections install …`,
`bouncers add …`, `hub upgrade`) **defines the whole process from env vars** —
container-supervisor creates a process that is not in the file, not only
overrides one that is — and re-adds the `crowdsec → cscli` wait so the item
loads in the same start (`SUPERVISOR_PROCESSES__CROWDSEC__DEPENDS_ON__CSCLI__EXIT=any`
merges into the existing `depends_on`). The README documents the full set.
`ARGUMENTS` splits on whitespace, so a comma-separated or JSON-looking value
arrives as one argument and `cscli` prints its help; number the entries
(`…__ARGUMENTS__0`, `__1`) for an argument that must contain a space.

The `depends_on` graph is validated before anything starts — a dangling
reference (`crowdsec` depending on a `cscli` that no longer exists) is a *fatal
startup error*, not a silent skip, which is why removing the slot means removing
the dependency in the same edit.

`hide_labels: true` drops the `[<process>]` prefix from child output, so
CrowdSec's log lines reach `docker logs` in stock format. The supervisor's own
lines keep the label.

No `setcap` anywhere: 8080 and 6060 are unprivileged.
