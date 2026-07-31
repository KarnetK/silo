# Kareg Silo TEST deployment

This profile runs the Kareg Silo release as an isolated, single-node local TEST
instance. It does not replace or modify the Portal MinIO container.

## Storage boundary

- Repository and Compose configuration: `E:\Git\Silo`.
- Persistent object data: `E:\SiloTest\data` by default.
- Docker Desktop may keep image layers and its Linux VM on drive `C:`. Those
  layers are reproducible caches; persistent Silo data must stay on drive `E:`.

The Compose file uses a bind mount instead of a Docker named volume so the data
location is explicit and survives Docker cache cleanup.

## Start

1. Generate the ignored local environment and data directory:

   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass `
     -File .\Initialize-TestDeployment.ps1
   ```

   The script creates a random local password without printing it. Existing
   `.env` files are retained unless `-Force` is specified. When an older file
   lacks the Portal, Synapse, or backup compatibility keys, only those missing
   deployment credentials are added; the existing Silo root pair is not
   rotated.

2. Keep `SILO_IMAGE` in `.env` pinned to an immutable release tag or image
   digest.
3. Run from this directory:

   ```powershell
   docker compose up -d --wait
   ```

The S3 API is available at `http://127.0.0.1:19000` and the administrative
console at `http://127.0.0.1:19001`. These ports avoid the Portal MinIO instance
on ports 9000 and 9001.

## Verify and stop

```powershell
docker compose ps
Invoke-WebRequest http://127.0.0.1:19000/minio/health/live -UseBasicParsing
docker compose down
```

`docker compose down` removes the container and network but leaves
`E:\SiloTest\data` intact. Removing that directory is a separate destructive
operation and is not part of the deployment workflow.

## Portal compatibility

The `.env` also contains generated local service-account pairs for Portal,
Synapse, and read-only backup verification. Bucket names, IAM policies, and the
isolation scenario remain owned by the Portal repository so the deployment
cannot drift from that product contract. Run its local Silo verifier after this
container is healthy.
