# coredns — build internals

CoreDNS built from source with the `acmednschallenge`, `traefik`, and `records`
plugins.

## Plugins

`plugins.json` is the single source of truth — it drives both what gets cloned
and the order of the plugin chain:

```json
[
  {
    "name": "records",
    "repo": "https://github.com/coredns/records",
    "ref": "a3157e710d9e57c75e4950a3750228f3ed9bb47a",
    "before": "forward"
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

- **acmednschallenge** — `before: traefik`. It intercepts only `_acme-challenge`
  TXT queries and passes everything else on, so it must run before **every**
  authoritative resolver, or that resolver answers the challenge query
  (NXDOMAIN/NODATA, no fallthrough) before acmednschallenge sees it — normal
  names still resolve, only issuance breaks. Its README says "above `file` and
  `forward`", but that predates carrying zone plugins (`traefik`, `records`)
  that sit *above* `file`. `traefik` is now the topmost such resolver, so
  acmednschallenge anchors directly above it; its `plugins.json` entry must
  therefore come *after* `traefik`, so the `traefik:` line already exists when
  this one is placed.
- **traefik** — `before: template`, above every backend that answers its zone
  (`template`, `hosts`, `file`, `auto`, `secondary`, `etcd`, `forward`). It
  serves hosts discovered from a Traefik instance and returns NXDOMAIN for any
  other name — no fallthrough unless configured — so any of those backends
  running first would answer a discovered host before `traefik` sees it.
- **records** — `before: forward`. It has no `fallthrough`, so wherever it sits
  it answers (or NXDOMAINs) authoritatively for its zones and ends the chain.
  Placed as low as possible — below every other authoritative zone plugin
  (`hosts`, `file`, `auto`, `secondary`, `etcd`), so each gets first refusal —
  but it must stay above the `forward` catch-all, which proxies everything and
  terminates the chain, so any resolver below `forward` never runs.

Anchor on the plugin the constraint actually names, not on a neighbour that
happens to sit in the right place — a neighbour can move upstream while still
resolving, silently drifting off the requirement. An anchor that isn't in
`plugin.cfg` at all fails the build rather than dropping the plugin silently.

`dnssec` needs no entry: upstream already places it above `hosts`/`file`, so it
sits above both and signs their responses, and leaving it below `cache`
means signed responses are cached instead of re-signed on every hit.

The resulting chain, upstream entries elided:

```
… cache … dnssec … acmednschallenge, traefik, template … hosts,
route53 … kubernetes, file, auto, secondary, etcd, loop, records, forward …
```

Adding a plugin means adding one object to `plugins.json` — no Dockerfile
change. CoreDNS itself is pinned by the `COREDNS_VERSION` build arg, set in
`docker-bake.hcl` (`v1.14.7`).

## Module overrides

`modules.json` is the second source of truth, for Go dependencies rather than
plugins: a list of `{module, min, advisory}` applied between `make gen` and
`go mod tidy`. It exists because CoreDNS releases pin dependency versions at
cut time — an advisory published afterwards leaves the Trivy gate failing with
no upstream release to bump to.

Each entry is a *floor*, not a pin. The build reads the currently selected
version with `go list -m` and only runs `go get $module@$min` when `min` sorts
higher (`sort -V`); otherwise it logs `override no longer needed` and moves on.
That is the whole point of the indirection — `go mod edit -require` and a plain
`go get module@version` both set the version exactly, so a stale entry would
silently *downgrade* the dependency the day CoreDNS ships something newer.
Entries are therefore safe to leave in place; delete them when the log says they
are inert.

`go list -m` fails the build for a module CoreDNS no longer requires, so an
entry cannot rot into a silent no-op. `modules.json` may be `[]`.

The override runs in its own `RUN` layer between two others that were one layer
before, since a `go get` failure and a compile failure want to be told apart.

## Startup: corefile-gen, then CoreDNS

Two release binaries sit next to `coredns`, both fetched from
`.../releases/download/$VERSION/<tool>-linux-<arch>`. The two repos name their
32-bit arm asset differently, so the suffix differs — no mapping table either
way, just different variables:

| Binary | Repo | Build arg | Asset suffix |
| --- | --- | --- | --- |
| `corefile-gen` | [coredns-envvar-corefile](https://github.com/BaseCrusher/coredns-envvar-corefile) | `COREFILE_GEN_VERSION` (`v1.1.0`, set in `docker-bake.hcl`) | `$TARGETARCH` — `amd64`, `arm64`, `arm` |
| `container-supervisor` | [container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` (`v1.10.0`) | `$TARGETARCH$TARGETVARIANT` — `amd64`, `arm64`, `armv7` |

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

Top-level `hide_labels: true` drops the `[<process>]` prefix the supervisor
otherwise puts on every line a child writes, so CoreDNS's own log lines reach
`docker logs` in stock CoreDNS format — anything parsing them does not have to
know the image runs a supervisor. It affects child output only; the supervisor's
own `[supervisor]` lines are unaffected, so a failed start is still identifiable.

Because `corefile-gen` always writes that path, a Corefile mounted at
`/home/nonroot/config/Corefile` is overwritten (or, mounted `:ro`, fails the
write and aborts the run) — env vars are now the way in.

CoreDNS is no longer PID 1, but `cap_net_bind_service` is a file capability on
the binary, so it still applies across the supervisor's `exec` and port 53
still binds without root.

### A-records helper (`a-records/`)

A third process, `a-records`, runs *before* `corefile-gen`. It exists because
`corefile-gen` renders one directive per env var: repeating an `A` record means
a hand-numbered `COREDNS_<GROUP>__records___AT__<N>` per IP. The helper lets you
pass the whole pool in one variable, `COREDNSARECORDS_<GROUP>=ip1,ip2,…`, and
expands it into those numbered vars.

It is a tiny Go program built from source in the build stage (`go build` into
`/coredns_temp/a-records`, so it ships next to `coredns` at `/home/nonroot`), not
a fetched release. Stdlib only; `main.go` keeps the transform in a pure `expand`
function with a test.

Why a separate process and not a `corefile-gen` fork: container-supervisor
v1.9.0+ passes env vars between processes through an `env_dir` (one file per var,
filename = key, read into every later child before it starts, config always
winning last). `a-records` writes the expanded `COREDNS_<GROUP>__records___AT__<N>`
files into `env_dir` (`/container-supervisor/supervisor_environment`, pre-created
`chown 65532` in the Dockerfile because the distroless final stage has no shell
to `mkdir`); `corefile-gen` `depends_on` it, so it sees them. `env_dir` did not
exist before container-supervisor v1.9.0.

The input lives in the `COREDNSARECORDS_` namespace, **outside** `COREDNS_`, on
purpose. `corefile-gen` reads only `COREDNS_*`, so it never sees the raw pool
variable — the helper is the sole reader, and there is no polluting directive to
suppress. `env_dir` can add and override keys but not *unset* one, so a marker
*inside* `COREDNS_` (e.g. a `_MULTIPLE` suffix) could not be hidden from
`corefile-gen` once corefile-gen v1.1.1 — which understood that suffix — was
withdrawn back to v1.1.0, which does not.

The helper is **disabled by default at the supervisor level**: the `a-records`
process is `enabled: false`, flipped on with
`SUPERVISOR_PROCESSES__A-RECORDS__ENABLED=true`. This works cleanly only because
`corefile-gen`'s dependency on it carries `ignore_exit_when_disabled: true`
(container-supervisor v1.10.0+): a disabled process otherwise reports `failure`
and, since `corefile-gen` requires it to exit `success`, would skip `corefile-gen`
and CoreDNS entirely. `ignore_exit_when_disabled` treats the condition as met
when `a-records` is disabled, so the rest of the chain runs untouched — hence the
`SUPERVISOR_VERSION` bump to `v1.10.0`. The process name carries the hyphen into
the `SUPERVISOR_PROCESSES__A-RECORDS__ENABLED` override; that env var name is fine
for `docker -e`/Compose, and on Kubernetes needs the
`RelaxedEnvironmentVariableValidation` feature gate (default-on in recent
releases). Enabled but given no `COREDNSARECORDS_<GROUP>` var, it writes nothing
and exits 0.

It appends after the highest existing index in each group (computed from the
`COREDNS_<GROUP>__records___AT__<N>` vars already in the environment, so static
SOA/NS records keep their slots) and emits `@ IN A <ip>` with no explicit TTL,
leaving the zone default to apply.

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
