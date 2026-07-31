# Kareg downstream maintenance

This repository is a fork of `pgsty/minio` used to preserve a controlled Silo
source and release path for Kareg deployments.

## Branches

- `master` follows `pgsty/minio:master` without Kareg-specific source changes.
- `kareg/main` contains only the release-pipeline changes required to publish
  Kareg-owned artifacts. Product source changes should be added only when a
  concrete compatibility or security defect requires them.

## Remotes

- `origin`: `https://github.com/KarnetK/silo.git`
- `upstream`: `https://github.com/pgsty/minio.git`

The local `upstream` remote is fetch-only. Upstream changes are reviewed and
merged into `kareg/main`; they are never pushed back to the upstream project by
the maintenance checkout.

## Releases

Release tags keep the upstream `RELEASE.YYYY-MM-DDTHH-MM-SSZ` format. The
release workflow builds binaries and packages for the supported platforms,
publishes the multi-architecture container image to
`ghcr.io/karnetk/silo`, and creates the matching GitHub release in
`KarnetK/silo`.

Each deployment must pin an immutable release tag or image digest. The
`latest` tag is a convenience pointer and is not a deployment contract.

## Local storage

The maintenance checkout lives at `E:\Git\Silo`. Heavy build caches, temporary
artifacts, and test data must remain on drive `E:`. Docker Desktop currently
stores its VM disk on drive `C:`, so container publication is performed by
GitHub Actions rather than by a local Docker build.
