# coredns — build internals

CoreDNS built from source with the `traefik`, `acmednschallenge` and `records`
plugins.

## Plugins

`plugins.json` is the single source of truth — it drives both what gets cloned
and the order of the plugin chain:

```json
[
  {
    "name": "traefik",
    "repo": "https://github.com/BaseCrusher/coredns-traefik",
    "ref": "v1.0.0",
    "before": "file"
  }
]
```

| Key | Meaning |
| --- | --- |
| `name` | Plugin name, and its `plugin/<name>/` directory in the CoreDNS tree. |
| `repo`, `ref` | Cloned at that ref into `plugin/<name>/`; the clone's own `go.mod`/`go.sum` are dropped so it builds as part of the CoreDNS module. Omit for a plugin already in the CoreDNS tree that only needs repositioning. |
| `before` / `after` | Insert the plugin's line directly above / below this plugin's line in `plugin.cfg`. Exactly one is required. |

`plugin.cfg` order is request-handling order, so placement is what makes these
plugins work at all. Each anchor is taken straight from the plugin's own README:

- **traefik** — `before: file`. Its README says to add it "above the backend
  plugins that answer the zone — `file`, `auto`, `secondary`, `etcd`, and
  `forward`", and recommends "just before `file`" since that clears all five.
- **acmednschallenge** — `before: file`. Its README says "above `file` and
  `forward`"; it only intercepts `_acme-challenge` TXT queries and passes
  everything else on, so a zone plugin running first breaks issuance.
- **records** — `after: hosts`. Its README states "the *host* plugin is
  configured before *records* in `plugin.cfg`, which means that when both are
  being specified in a server block, the *host* plugin will get preference."

Anchor on the plugin the constraint actually names, not on a neighbour that
happens to sit in the right place — a neighbour can move upstream while still
resolving, silently drifting off the requirement. An anchor that isn't in
`plugin.cfg` at all fails the build rather than dropping the plugin silently.

`dnssec` needs no entry: upstream already places it above `hosts`/`file`, so it
sits above all three and signs their responses, and leaving it below `cache`
means signed responses are cached instead of re-signed on every hit.

The resulting chain, upstream entries elided:

```
… cache … dnssec … hosts, records, route53 … kubernetes,
traefik, acmednschallenge, file, auto, secondary, etcd, loop, forward …
```

Adding a plugin means adding one object to `plugins.json` — no Dockerfile
change. CoreDNS itself is pinned by the `COREDNS_VERSION` build arg
(default `v1.14.6`).

## Startup: corefile-gen, then CoreDNS

Two release binaries sit next to `coredns`, both fetched from
`.../releases/download/$VERSION/<tool>-linux-<arch>`. The two repos name their
32-bit arm asset differently, so the suffix differs — no mapping table either
way, just different variables:

| Binary | Repo | Build arg | Asset suffix |
| --- | --- | --- | --- |
| `corefile-gen` | [coredns-envvar-corefile](https://github.com/BaseCrusher/coredns-envvar-corefile) | `COREFILE_GEN_VERSION` (`v1.0.1`) | `$TARGETARCH` — `amd64`, `arm64`, `arm` |
| `container-supervisor` | [container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` (`v1.0.2`) | `$TARGETARCH$TARGETVARIANT` — `amd64`, `arm64`, `armv7` |

`$TARGETVARIANT` is empty for `linux/amd64` and `linux/arm64` (buildx
normalises `arm64/v8` to an empty variant) and `v7` for `linux/arm/v7`, so the
concatenation is exactly the asset name in all three cases. container-supervisor
published no 32-bit arm build before v1.0.2.

`corefile-gen` writes the Corefile from `COREDNS_*` env vars to the path it is
given — a real file, no shell redirection, so it works in a distroless image.
It does not exec CoreDNS afterwards, so something has to sequence the two: the
entrypoint is `container-supervisor`, with `supervisor.yml` baked in at its
default config path `/container-supervisor/config.yml`:

- `corefile-gen` — `one_shot`. `on_failure` is left at its default `fail`: if
  the Corefile can't be written the container aborts rather than starting
  CoreDNS against a stale or missing one.
- `coredns` — `service`, `depends_on: corefile-gen: {exit: success}`. A
  `service` may only `depends_on` from container-supervisor v1.0.1 onwards;
  v1.0.0 rejects that config at load with a fatal error.

Because `corefile-gen` always writes that path, a Corefile mounted at
`/home/nonroot/config/Corefile` is overwritten (or, mounted `:ro`, fails the
write and aborts the run) — env vars are now the way in.

CoreDNS is no longer PID 1, but `cap_net_bind_service` is a file capability on
the binary, so it still applies across the supervisor's `exec` and port 53
still binds without root.

## Image layout

Runs as `nonroot` (uid/gid 65532) on `gcr.io/distroless/static-debian13`, with
`cap_net_bind_service` set on the binary so it can bind port 53 without root.
No shell, no package manager — the entrypoint execs the binary directly.

The final stage is `FROM $BASE_IMAGE`, a global build arg, which is the only
difference between the two published images: the `coredns-debug` bake target
overrides it with `:debug-nonroot` and appends `-debug` to the tag. Everything
before it is the same build stage, so both images ship identical binaries.

Both are tagged `${COREDNS_VERSION}` (plus a `latest` that follows `main`),
and the bake file passes that same variable as the build arg — the tag and what's inside cannot drift. Bumping CoreDNS means
editing `docker-bake.hcl` (the Dockerfile default only applies to a bare
`docker build`, which produces no tag).

## Cross-compilation

The build stage is pinned to `$BUILDPLATFORM` and Go is pointed at
`$TARGETOS`/`$TARGETARCH`/`$TARGETVARIANT`, so adding a platform to
`docker-bake.hcl` is enough — no emulation needed.
