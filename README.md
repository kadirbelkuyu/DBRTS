# Database Restore Transfer System

Database Restore Transfer System is the terminal companion we use to shuttle data between PostgreSQL and MongoDB estates, capture backups, and inspect servers. It exposes the exact workflows our automation uses, wrapping them in a full-screen prompt loop so you can operate everything without memorising flags.

## Features

- **Multi-engine support** – run the same commands against PostgreSQL or MongoDB by switching configuration files.
- **Transfer pipelines** – migrate schemas and data with batching, worker pools, and progress feedback (PostgreSQL) or clone collections with index replication (MongoDB).
- **Backup & restore orchestration** – wrap `pg_dump`/`pg_restore` and `mongodump`/`mongorestore`, capture metadata, calculate checksums, and store artifacts under `backup/`.
- **Interactive mode** – launch `dbrts interactive` to drive transfers, backups, restores, or listings via guided prompts.
- **Connection profiles** – every config you enter can be saved and recalled from the wizard; no more copy/pasting file paths.
- **Live explorer** – open a TUI with `dbrts explore` to browse tables or collections, preview rows/documents, and run ad-hoc SQL or MongoDB JSON commands without leaving the terminal.
- **Verbose logging & progress bars** – toggle rich diagnostics and monitor long-running jobs directly from the terminal.

## Requirements

Install the native tooling for the engines you plan to use:

### PostgreSQL

- `pg_dump`, `pg_restore`, and `psql` (bundled with PostgreSQL or available via `libpq` packages).
- Ensure the binaries are on your `PATH`.

### MongoDB

- `mongodump` and `mongorestore` from the MongoDB Database Tools distribution.
- Ensure the binaries are on your `PATH`.
- You can install them via `scripts/install-mongodb-tools.sh`.

Verify the setup:

```bash
pg_dump --version
pg_restore --version
psql --version
mongodump --version
mongorestore --version
```

## Build

```bash
# Download dependencies and tidy the module
go mod tidy

# Build the cli
go build -o bin/dbrts ./cmd/dbrts

# Optional make targets
make deps   # install Go toolchain dependencies
make build  # compile into ./bin
```

### Run the CLI

```bash
# Option 1: use the Makefile helper
make run          # builds then launches the interactive wizard

# Option 2: run the binary directly
./bin/dbrts interactive

# Inspect available commands/flags
./bin/dbrts --help
```

## Usage

Start with the interactive path if you’re onboarding a new environment:

```bash
./bin/dbrts interactive
```

When you start the app you land on the interactive screen in the screenshot above. The loop lists any saved configs under `configs/`, or prompts for connection details and persists them automatically so you can reuse them later. If you’d rather script things or run Database Restore Transfer System in CI, drive the Cobra commands directly.

> **Note:** Every command requires that the source/target configs describe the same engine—Database Restore Transfer System intentionally blocks cross-engine transfers.

### Transfer data

```bash
# Full transfer (schema + data) between matching engines
./bin/dbrts transfer \
  --source-config configs/source-postgres.yaml \
  --target-config configs/target-postgres.yaml \
  --workers 8 \
  --batch-size 500 \
  --verbose

# Schema-only transfer
./bin/dbrts transfer \
  --source-config configs/source-postgres.yaml \
  --target-config configs/target-postgres.yaml \
  --schema-only

# Data-only transfer for MongoDB (copies collections + documents)
./bin/dbrts transfer \
  --source-config configs/source-mongo.yaml \
  --target-config configs/target-mongo.yaml \
  --data-only
```

> **Cross-engine transfers (PostgreSQL ↔ MongoDB)** are intentionally blocked. The source and target types must match.

### Create a backup

```bash
# PostgreSQL backup with interactive format selection
./bin/dbrts backup --config configs/source-postgres.yaml --verbose

# MongoDB backup (archives stored under backup/)
./bin/dbrts backup --config configs/source-mongo.yaml
```

### Restore a backup

```bash
./bin/dbrts restore --config configs/target-postgres.yaml
./bin/dbrts restore --config configs/target-mongo.yaml --verbose
```

### List databases on a server

```bash
./bin/dbrts list-databases --config configs/source-postgres.yaml
./bin/dbrts list-databases --config configs/source-mongo.yaml
```

### Explore a database interactively

`dbrts explore` opens a split-view console: the left pane lists tables (PostgreSQL) or collections (MongoDB), the top-right pane shows live data, and the bottom-right pane displays metadata.

1. Launch the explorer with a config that points to the engine you want to inspect.
2. Use the arrow keys to move through tables or collections; the preview refreshes automatically.
3. Press `:` to open the command palette, `r` to refresh the active preview, or `q` to exit.

```bash
# PostgreSQL schema explorer
./bin/dbrts explore --config configs/source-postgres.yaml

# MongoDB collection explorer
./bin/dbrts explore --config configs/source-mongo.yaml
```

Preview queries fetch up to 200 rows for PostgreSQL and 50 documents for MongoDB, keeping the interface responsive while still surfacing enough data to audit what is stored.

#### Command palette

Inside the explorer, hit `:` to execute write/read operations without leaving the TUI.

- **PostgreSQL** — type any SQL statement. `SELECT` queries render directly in the preview (trimmed to 200 rows). `INSERT`, `UPDATE`, and `DELETE` run immediately; the footer reminds you to hit `r` if you want to refresh the current table afterwards.
- **MongoDB** — use verb-prefixed JSON commands:
  - `insert {"name":"alpha","status":"active"}`
  - `update {"filter":{"_id":{"$oid":"..."}}, "update":{"$set":{"status":"archived"}}}`
  - `delete {"filter":{"status":"temp"}}`
  - `find {"status":"active"}` (leave the payload empty to search with an empty filter)

Payloads accept Extended JSON (e.g., `$oid`, `$date`). The explorer validates input before executing anything against the database.

## Configuration

### Saved configs

The interactive wizard reads everything under `configs/` via the profile manager and lists the aliases alongside their engine type. Pick a number to reuse an existing profile or choose `n` to define a new one. After you fill in the prompts, the wizard offers to save the profile back to disk (you can override the suggested alias). Those YAML files remain fully editable in Git.

### Manual YAML

If you prefer to manage configs in Git, create YAML files describing the target servers. The CLI honours `database.type` to decide which adapter (PostgreSQL or MongoDB) to use. For MongoDB clusters hosted on Atlas/DigitalOcean/etc., you can place the `mongodb+srv://` URI straight into `database.uri` and omit host/port.

### PostgreSQL example

```yaml
# configs/source-postgres.yaml
database:
  type: postgres
  host: localhost
  port: 5432
  database: mydb
  username: postgres
  password: password
  sslmode: disable
```

### MongoDB example

```yaml
# configs/source-mongo.yaml
database:
  type: mongo
  host: localhost
  port: 27017
  database: mydb
  username: mongoUser        # optional when auth disabled
  password: mongoPass        # optional when auth disabled
  auth_database: admin       # optional auth DB
  uri: ""                    # optional: override host/port with a full MongoDB URI
```

When `uri` is present it takes precedence over the other Mongo connection attributes.

## Development Notes

- `go test ./...` builds all packages; integration suites under `tests/` rely on Docker and Testcontainers and may require a running Docker daemon.
- The interactive helpers reuse the same Cobra commands, so any future flag updates automatically flow through the wizard.
- Backups are written to `backup/` by default; ensure the process has write permissions.

## Troubleshooting

- **Missing tooling** – If `pg_dump`/`mongodump` (or related utilities) are not on the `PATH`, the backup service fails with a descriptive error. Install the respective client tools and retry.
- **Permission errors** – Ensure the configured database credentials have sufficient privileges to list databases, run dumps, and create tables/collections.
- **Long running transfers** – Use `--workers` and `--batch-size` to tune throughput on large data sets or rely on the progress bar feedback to monitor operations.
- **DNS issues (MongoDB)** – If you connect to a Kubernetes or cloud cluster via SRV, make sure your terminal can resolve the service name. When in doubt, port-forward or supply a reachable host/IP.

Happy shipping! 🚀
