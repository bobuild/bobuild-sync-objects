# bobuild-sync-objects

CLI tool that keeps Bobuild object data in sync across apps using the [Bobuild REST API](https://docs.bobuild.com/advanced/api/).

Typical use: one **source** app holds the master records; one or more **target** apps receive a full replica of selected objects (create missing rows, update changed rows, delete extras), matched by `id`.

It is built on [`bobuild-client-go`](https://github.com/bobuild/bobuild-client-go).

## How it works

For each configured **job**:

1. Load every listed table from the source (`GET`).
2. For each target, in table order (respect foreign keys):
   - load the target list (`GET`)
   - **create** source rows missing on the target (`POST` non-file fields, then per-file `PATCH` uploads)
   - **update** rows whose non-file payload differs (`PATCH` non-file fields, then per-file uploads)
3. Then, in **reverse** table order, **delete** target rows whose `id` is not on the source (`DELETE /{id}`).

Source and target must expose the **same JSON field names** (`variable` on the endpoint Fields tab). If the underlying objects use different field names, map them in the Builder — this tool does not remap fields.

Comparison is structural equality of the JSON objects (with light normalization of number types), **excluding** configured `file_fields` (signed URLs differ per app). Identical non-file rows are skipped; files are uploaded only when a create or non-file update runs.

Failures on one job, table, or target are isolated; other work continues. The process exits non-zero if any error occurred.

## Bobuild setup

API feature must be enabled on each app. See [API documentation](https://docs.bobuild.com/advanced/api/) for endpoint types, authentication, and field mapping.

### 1. API keys

In each app: **Settings → API → API Keys** → create a key (e.g. `object-sync`).  
On every sync endpoint: **Authentication** → method **API Key** → allow that key.  
Calls use `Authorization: Bearer <api_key>`.

### 2. Endpoint convention

Pick a path prefix per app (e.g. `/sync`). For every table name `T` in the job:

| App | Path | Action | HTTP |
|-----|------|--------|------|
| Source | `{prefix}/T` | List | `GET` |
| Target | `{prefix}/T` | List | `GET` |
| Target | `{prefix}/T` | Create | `POST` |
| Target | `{prefix}/T/{id}` | Modify | `PATCH` |
| Target | `{prefix}/T/{id}` | Delete | `DELETE` |

Configure the same set of API variables on source list and target list/create/modify so payloads round-trip. On **Modify**, do **not** map `id` — Bobuild takes it from the URL path and rejects `id` in the body; this tool strips `id` from PATCH payloads automatically.

**Preserve IDs:** map `id` as an accepted field on the target **Create** endpoint so replicas keep the source primary key. Without that, creates get new IDs and the sync cannot match rows.

**File / image fields:** list them under `file_fields` for that table. Map those fields on the target **Modify** endpoint (Create need not require them). List returns temporary URLs; this tool downloads each URL and PATCHes Bobuild’s `{ data: base64, name }` shape [one file at a time](https://docs.bobuild.com/advanced/api/).

Order tables in config so parents are listed before children (creates/updates). Deletes automatically run in reverse order.

### 3. Optional hardening

- [Allowed source IPs](https://docs.bobuild.com/advanced/api/) on modify/delete endpoints  
- Separate keys per integration  
- [OpenAPI documents](https://docs.bobuild.com/advanced/api/) for the sync surface

## Config

```bash
cp config.example.yaml config.yaml
# edit jobs, domains, api_key, path_prefix
```

Do not commit `config.yaml` with secrets.

```yaml
max_file_size_mb: 100   # optional; default 100

jobs:
  - name: catalog
    tables:
      - category   # parents first
      - name: product
        file_fields: [cover, datasheet]
    source:
      domain: source.example.com
      api_key: ${SYNC_SOURCE_KEY}
      path_prefix: /sync
    targets:
      - domain: replica.example.com
        api_key: ${SYNC_REPLICA_KEY}
        path_prefix: /sync
```

`api_key` may be a literal string (fine for local use) or `${ENV_VAR}` as the whole value. Each key can use its own env var name. Unset or empty variables fail at startup with a clear error. In containers (e.g. AWS ECS/Fargate), inject secrets as environment variables from Secrets Manager or SSM — one env var per API key referenced in the config.

Optional global:
- `max_file_size_mb` — max download size per file field (default `100`). Larger files log an error and skip that record (avoids container OOM).

Optional per app:
- `host` / `port` — dial a cluster ingress over plain HTTP while keeping `domain` as the HTTP Host (default port `80` if only `host` is set)
- `insecure_skip_verify: true` — skip TLS certificate verification (private CA / broken chain). Prefer fixing trust store; only enable for known endpoints. On the **source** app this also applies to downloading file/image URLs returned by List.

Tables may be a bare name (`category`) or a mapping with `file_fields`.

## Usage

```bash
go run ./cmd/sync -config config.yaml
go run ./cmd/sync -config config.yaml -dry-run
go run ./cmd/sync -config config.yaml -job catalog
go run ./cmd/sync -config config.yaml -source source.example.com -target replica.example.com
```

Build a binary:

```bash
go build -o bobuild-sync-objects ./cmd/sync
./bobuild-sync-objects -config config.yaml
```

### Flags

| Flag | Description |
|------|-------------|
| `-config` | Path to YAML config (default `config.yaml`) |
| `-dry-run` | Log would-be create/update/delete without writing |
| `-job` | Run a single job by name |
| `-source` | Only jobs whose source `domain` matches |
| `-target` | Only that target `domain` within selected jobs |

## Limitations

- Full reconcile only (no incremental / watermark sync).
- Same API contract on source and target; no field transforms in this tool.
- Table order for FK safety is your responsibility.
- File/image fields must be listed in `file_fields`; they are ignored in equality checks, so a file-only change on the source is not detected until some non-file field changes (or the row is created).
- Files are uploaded sequentially (one in memory at a time); oversized files (`max_file_size_mb`) skip the record.
- Target Create must accept client-supplied `id` for stable replication.

## License

MIT — see [LICENSE](LICENSE).
