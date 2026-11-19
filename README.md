# GFT Dry-Run — SQS Consumer in Go

An ingestion pipeline that processes two event streams — **Clients** and **Blocks** — from SQS into DynamoDB, deployed as AWS Lambda functions and provisioned with Terraform.

---

## Architecture

```
         SQS                 Lambda (Go)             DynamoDB
  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────────┐
  │ cadastro-       │───▶│ cadastro-clientes │───▶│                  │
  │ clientes        │    │  DOMAIN=cliente   │    │  cadastro_geral  │
  └────────┬────────┘    └──────────────────┘    │  (PK: codigo_    │
           │ DLQ                                  │   cliente)       │
           ▼                                      │                  │
  ┌─────────────────┐    ┌──────────────────┐    │                  │
  │ cadastro-       │───▶│ cadastro-bloqueios│───▶│                  │
  │ bloqueios       │    │  DOMAIN=bloqueios │    └──────────────────┘
  └────────┬────────┘    └──────────────────┘
           │ DLQ                │ failed msgs
           ▼                    ▼
      (after 3           falhas_processamento
      attempts)          (PK: id_mensagem)
```

Both Lambda functions use the **same compiled binary**. The active domain is selected via the `DOMAIN` environment variable set by Terraform.

---

## Prerequisites

| Tool | Minimum version | Notes |
|---|---|---|
| Docker + Docker Compose | v2.x | Required for the local emulator |
| Go | 1.27 | Only for `make build`; not needed for `make build-docker` |
| Terraform | 1.6 | For `make deploy` |
| AWS CLI | v2 | For `make demo` |

---

## Quick Start (local demo)

```bash
# 1. Start the local AWS emulator (LocalStack/Floci)
make up

# 2. Build the Lambda binary + zip
make build

# 3. Deploy all infrastructure with Terraform
make deploy

# 4. Run demo scenarios and inspect the result
make demo
```

Or all at once:

```bash
make all        # up → build → deploy
make demo
```

To tear everything down:

```bash
make destroy    # terraform destroy + docker compose down
```

---

## Project Structure

```
.
├── cmd/lambda/main.go          Entry point: wires repo + handler, starts Lambda runtime
├── internal/
│   ├── domain/
│   │   ├── client.go           ClientMessage type, ParseClientMessage, IsNewerThan
│   │   ├── block.go            BlockMessage, Block types, ParseBlockMessage, IsNewerThan
│   │   └── errors.go           PermanentError, TransientError
│   ├── handler/
│   │   ├── handler.go          SQS batch handler: parse, classify, fan-out
│   │   └── batch.go            Group by client, sort by timestamp, worker pool
│   └── repository/
│       ├── dynamo.go           DynamoRepository, Repository interface, Config, helpers
│       ├── client.go           SaveClient: GetItem → compare → PutItem/UpdateItem
│       ├── block.go            SaveBlocks: same pattern for the blocks domain
│       └── failure.go          FailureRecord type + SaveFailure
├── infra/
│   ├── provider.tf             AWS provider (endpoint override for local emulator)
│   ├── variables.tf            Input variables with defaults
│   ├── locals.tf               visibility_timeout, arch mapping
│   ├── sqs.tf                  4 queues: 2 main + 2 DLQs
│   ├── dynamodb.tf             cadastro_geral + falhas_processamento
│   ├── iam.tf                  2 roles, minimal permissions per Lambda
│   ├── lambda.tf               2 functions, same zip, different DOMAIN env var
│   ├── esm.tf                  2 Event Source Mappings (ReportBatchItemFailures)
│   └── outputs.tf              Queue URLs, table name, Lambda ARNs
├── Dockerfile                  Multi-stage build → lambda.zip (no local Go required)
├── docker-compose.yaml         LocalStack/Floci emulator with healthcheck
└── Makefile                    All workflow targets (see below)
```

---

## Make Targets

```
make all            Full pipeline: up → build → deploy
make up             Start the local emulator, wait for healthcheck
make down           Stop the emulator and remove volumes
make build          Compile for Linux + zip (requires local Go)
make build-docker   Same result via Docker BuildKit (no local Go needed)
make deploy         terraform init + apply against the running emulator
make destroy        terraform destroy + make down
make test           go test ./... -count=1
make demo           Send sample messages, wait, print DynamoDB result + DLQ count
make logs           Stream emulator container logs
make fmt            gofmt -w . + terraform fmt
make validate       go vet + terraform validate
make help           Show all targets
```

Override variables on the command line:

```bash
make build ARCH=arm64
make deploy ENDPOINT=http://localhost:4566 REGION=sa-east-1
make all ARCH=arm64 REGION=sa-east-1
```

---

## Configuration

### Makefile variables

| Variable | Default | Description |
|---|---|---|
| `ARCH` | `amd64` | Go build target arch (`amd64` or `arm64`) |
| `ENDPOINT` | `http://localhost:4566` | AWS endpoint override (empty = real AWS) |
| `REGION` | `us-east-1` | AWS region |

### Terraform variables (`infra/variables.tf`)

| Variable | Default | Description |
|---|---|---|
| `aws_endpoint` | `""` | Emulator endpoint; empty targets real AWS |
| `lambda_arch` | `amd64` | Must match the binary in the zip |
| `lambda_timeout` | `30` | Seconds; visibility_timeout = 6× this |
| `lambda_zip_path` | `../build/lambda.zip` | Path to the compiled zip |
| `table_name` | `cadastro_geral` | Main DynamoDB table |
| `failures_table_name` | `falhas_processamento` | Failure records table |

### Lambda environment variables (set by Terraform)

| Variable | Value |
|---|---|
| `DOMAIN` | `cliente` or `bloqueios` |
| `TABLE_NAME` | `cadastro_geral` |
| `FAILURES_TABLE` | `falhas_processamento` |
| `AWS_REGION` | Region from Terraform variable |
| `QUEUE_NAME` | Queue name (for FailureRecord) |
| `DYNAMODB_ENDPOINT` | Emulator endpoint or empty for real AWS |

---

## SQS Message Contracts

### Client domain (`DOMAIN=cliente`)

```json
{
  "codigo_cliente": "550e8400-e29b-41d4-a716-446655440000",
  "numero_agencia": 1,
  "numero_conta": 123,
  "numero_dac": 5,
  "data_atualizacao": "2024-01-15T10:00:00Z"
}
```

### Blocks domain (`DOMAIN=bloqueios`)

```json
{
  "codigo_cliente": "550e8400-e29b-41d4-a716-446655440000",
  "codigo_bloqueio_1": 100,
  "valor_bloqueio_1": "150.50",
  "codigo_bloqueio_2": 200,
  "valor_bloqueio_2": "75.00",
  "data_atualizacao": "2024-01-15T10:30:00Z"
}
```

Block pairs are numbered sequentially (`_1`, `_2`, …). Maximum 100 blocks per message.

---

## Key Design Decisions

| # | Decision | Why |
|---|---|---|
| 1 | Per-domain timestamp (`data_atualizacao_cliente` / `data_atualizacao_bloqueios`) | Domains operate independently; one domain must not block the other from writing a newer value |
| 2 | Shared `version` counter (optimistic locking) | A single row per client means both domains compete for the same lock; version increments on every write regardless of domain |
| 3 | Internal retry on `ConditionalCheckFailedException` (3 attempts, exp. backoff + jitter) | Transient version conflicts between the two domains are expected and should be absorbed without reaching SQS |
| 4 | Older or equal timestamp → discard with success | Makes SQS redeliveries idempotent without extra storage |
| 5 | Batch grouped by `codigo_cliente`: parallel groups, sequential within a group | Messages for different clients are independent; messages for the same client must be applied in timestamp order |
| 6 | Partial batch response (`ReportBatchItemFailures`) | Only failing messages are redriven; the rest are not penalized |
| 7 | `maxReceiveCount = 3` on SQS | 1 attempt + 2 redrives; on exhaustion the message goes to the DLQ and a record is written to `falhas_processamento` |
| 8 | Visibility timeout = 6 × Lambda timeout | Prevents a message from becoming visible again while still being processed |
| 9 | Decimal values stored as `json.Number` (string) | Avoids float precision loss; DynamoDB `N` attributes are strings internally anyway |

---

## Demo Scenarios

`make demo` covers scenarios 1–4 and 7. For the full set:

| # | Scenario | How to trigger |
|---|---|---|
| 1 | New client | `make demo` |
| 2 | Blocks before client exists | `make demo` |
| 3 | Older message discarded | `make demo` |
| 4 | Block list replaced and sorted | `make demo` |
| 5 | Domains don't overwrite each other | Send a client message, then a blocks message; verify each domain only touches its own columns |
| 6 | Concurrent domain conflict + internal retry | Send both domains simultaneously for the same GUID; check `version` increments twice |
| 7 | Batch with one invalid message | `make demo` (invalid GUID sent; only that message goes to DLQ) |
| 8 | >100 blocks rejected | Send a message with 101 `codigo_bloqueio_N` / `valor_bloqueio_N` pairs |
| 9 | Retries exhausted → DLQ + failure record | Send an unparseable message 3 times; check `falhas_processamento` table |

---

## Troubleshooting

**`make up` hangs / healthcheck never passes**

The emulator container is not starting. Check:

```bash
docker compose logs
docker ps -a
```

If the port is already in use:

```bash
# Change the port
EMULATOR_PORT=4567 make up
make deploy ENDPOINT=http://localhost:4567
```

**`make deploy` fails with "no such file: ../build/lambda.zip"**

Build the binary first:

```bash
make build        # requires local Go 1.27
# or
make build-docker # requires only Docker
```

**`terraform validate` fails with provider errors**

Run `terraform -chdir=infra init` once to download the AWS provider before validating.

**`make demo` shows no items in DynamoDB**

The Lambda has not processed the messages yet. Increase the sleep, or check the emulator logs:

```bash
make logs
```

---

## Plan B — LocalStack

The default emulator image in `docker-compose.yaml` is `localstack/localstack:3`, which is fully compatible with the same Terraform and AWS CLI commands. No changes to any code or configuration are needed.

To explicitly use the community edition:

```bash
EMULATOR_IMAGE=localstack/localstack:3 make up
```

> **Note:** Local emulators do not enforce IAM policies or account-level quotas. For final validation against real service limits, deploy to a real AWS account using `make deploy ENDPOINT= REGION=<your-region>`.

---

## Running on arm64 (Apple Silicon / Graviton)

```bash
make all ARCH=arm64
```

This sets `GOARCH=arm64` for the build and `lambda_arch=arm64` for Terraform, which maps to the `arm64` Lambda architecture. Everything else is identical.
