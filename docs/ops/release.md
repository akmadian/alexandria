# Release pipeline

Spec for implementation, once packaging (Wails) is actually wired in. Not
active yet — no Wails code in the repo. Written now so the design exists
before it's needed.

## Trigger

Tag push, not a dedicated release branch — less orchestration, no risk of a
long-lived branch drifting from `dev`/`main`.

```yaml
name: release
on:
  push:
    tags:
      - 'v*'
```

`git tag v1.2.3 && git push origin v1.2.3` is what fires it. No separate
"create release" step.

## Build

Wails builds are OS-native (can't cross-compile a macOS `.app` from Linux),
so a build matrix, one job per target OS, all triggered by the same tag:

```yaml
jobs:
  release:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        include:
          - os: macos-latest
            platform: darwin/universal
          - os: ubuntu-latest
            platform: linux/amd64
          - os: windows-latest
            platform: windows/amd64
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod, cache: true }
      - uses: actions/setup-node@v4   # or bun setup, matches frontend tooling
      - run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
      - run: wails build -platform ${{ matrix.platform }}
      - run: sha256sum build/bin/* > build/bin/SHA256SUMS
      - uses: anchore/sbom-action@v0
        with:
          path: build/bin
          format: spdx-json
          output-file: build/bin/sbom.spdx.json
      - uses: softprops/action-gh-release@v2
        with:
          files: build/bin/*
          generate_release_notes: true
```

- `softprops/action-gh-release` (community action, de facto standard —
  GitHub itself has no first-party "create release" action) creates the
  GitHub Release from the tag and attaches each matrix job's artifacts.
- `generate_release_notes: true` auto-populates the changelog from commits
  since the last tag — no extra changelog tooling needed.
- `SHA256SUMS` sidecar file is required for the updater's verify step (below)
  — generate it as part of the same job, not a separate workflow.
- `anchore/sbom-action` (wraps `syft`) generates an SPDX-format SBOM of the
  built artifact, attached to the release the same way as `SHA256SUMS` — see
  the SBOM section below for what it's for and where else it's used.

## Updater

GitHub Releases already acts as the manifest — no separate hosting needed.

- **Version check**: backend embeds its build version via `-ldflags` at
  build time (standard Go practice), compares against
  `GET https://api.github.com/repos/<org>/<repo>/releases/latest`.
- **Download + verify**: fetch the matching platform asset, verify against
  `SHA256SUMS` *before* touching the running binary — non-negotiable, an
  unverified binary swap is a supply-chain hole. This is a **free** check —
  sign `SHA256SUMS` with a self-owned Ed25519 keypair (e.g. `minisign`, or
  Go's stdlib `crypto/ed25519` directly) in the release workflow, embed the
  public key in the app binary at build time, verify the signature before
  swap. No CA, no cost — this is the actual security boundary the updater
  needs, and it doesn't require OS-level trust at all.
- **Swap**: atomic rename over the old binary, not in-place overwrite, so a
  crash mid-write can't corrupt the install. Prompt user to restart.
- Library: [`creativeprojects/go-selfupdate`](https://github.com/creativeprojects/go-selfupdate)
  targets exactly this GitHub-Releases-as-source pattern with checksum
  validators built in — use it rather than hand-rolling download+verify+swap.

## SBOM (software bill of materials)

A machine-readable manifest of every dependency (and transitive dependency)
actually baked into the built artifact — package, exact version, license,
sometimes a hash. The point is answerability later: when a CVE surfaces for
something three levels deep in the dependency tree, grep the SBOM for the
affected release instead of reconstructing "what was in v1.2.3" from memory.

Two places it lives, both wired into the release job above:

- **Attached to the GitHub Release** as `sbom.spdx.json`, same as
  `SHA256SUMS` — anyone downloading a release can pull it down alongside the
  binary.
- **Submitted to GitHub's Dependency Submission API**, feeding the repo's
  Security → Dependency graph and its CVE cross-referencing — the same
  Dependabot-alerts pipeline already enabled, but now covering what's
  actually *compiled into the shipped binary* rather than only what
  `go.mod`/`package.json` declare. Matters specifically for a Wails app:
  CGO-linked C libraries or anything statically bundled can end up in the
  binary without showing up plainly in the source manifests; the SBOM
  catches what dependency-graph parsing alone would miss.

### Wails version note

This whole updater section is a **Wails v2** design. Wails v3 (still alpha as
of mid-2026, no fixed beta date) ships a built-in updater with delta/bsdiff
patch downloads — if/when the project moves to v3, drop `go-selfupdate`
entirely and use the native one instead. Re-evaluate v3 readiness when
Wails is actually being wired into the app, not before (see
[v3alpha.wails.io/status](https://v3alpha.wails.io/status/)).

## Explicitly out of scope for now

- Delta/patch updates (download only the diff) — Wails v3 native feature,
  not worth building against v2.
- **OS-level code signing / notarization** (macOS notarization, Windows
  Authenticode) — this is cosmetic trust (removes the "unidentified
  developer"/SmartScreen warning), not a security boundary the updater
  depends on, and it costs real money: macOS notarization requires an Apple
  Developer Program account ($99/yr, no free tier, no way around it — a
  self-signed cert doesn't notarize). Windows self-signed certs are free but
  don't build SmartScreen reputation, so users still see a warning; the
  cheapest *legitimate* option is Azure Trusted Signing (cloud-based,
  region-restricted as of mid-2026) at a fraction of a traditional EV cert's
  cost. Acceptable interim state: ship unsigned, let users right-click→Open
  on macOS / click through SmartScreen on Windows. Revisit only once
  non-technical users are actually hitting this friction, not before.
- Version-bump automation (auto-generating the next tag from commits) — pure
  convenience, add if manual tagging becomes annoying.
