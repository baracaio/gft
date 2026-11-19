# ============================================================
# Configuration — override on the CLI: make build ARCH=arm64
# ============================================================
ARCH     ?= amd64
ENDPOINT ?= http://localhost:4566
REGION   ?= us-east-1

BUILD_DIR := build
ZIP       := $(BUILD_DIR)/lambda.zip

TF        := terraform -chdir=infra
TF_VARS   := -var="aws_endpoint=$(ENDPOINT)" \
             -var="lambda_arch=$(ARCH)"       \
             -var="aws_region=$(REGION)"

# A client GUID used in demo scenarios.
DEMO_CLIENT := 550e8400-e29b-41d4-a716-446655440000

AWS_LOCAL := aws --endpoint-url=$(ENDPOINT) --region=$(REGION)

.PHONY: all up down build build-docker deploy destroy test demo logs fmt validate help

# ============================================================
# Composite targets
# ============================================================
all: up build deploy   ## Full pipeline: start emulator → build → deploy

# ============================================================
# Emulator
# ============================================================
up:   ## Start the local AWS emulator and wait for it to be healthy
	docker compose up -d --wait
	@echo "Emulator ready at $(ENDPOINT)"

down: ## Stop the emulator and remove volumes
	docker compose down -v

# ============================================================
# Build
# ============================================================
build: ## Build the Lambda binary locally and package it into $(ZIP)
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 \
		go build -tags lambda.norpc -ldflags="-s -w" \
		-o $(BUILD_DIR)/bootstrap ./cmd/lambda
	cd $(BUILD_DIR) && zip -j lambda.zip bootstrap
	@echo "Built: $(ZIP)  (arch=$(ARCH))"

build-docker: ## Build the Lambda zip inside Docker — no local Go required
	DOCKER_BUILDKIT=1 docker build \
		--output $(BUILD_DIR)/ \
		--build-arg GOARCH=$(ARCH) \
		.
	@echo "Built: $(ZIP)  (arch=$(ARCH), via Docker)"

# ============================================================
# Infra
# ============================================================
deploy: ## Init + apply Terraform against the running emulator
	$(TF) init -input=false -upgrade -reconfigure
	$(TF) apply -auto-approve $(TF_VARS)
	@echo "Infra deployed"

destroy: ## Destroy Terraform resources then stop the emulator
	$(TF) destroy -auto-approve $(TF_VARS) || true
	$(MAKE) down

# ============================================================
# Quality
# ============================================================
fmt: ## Auto-format Go and Terraform files in-place
	gofmt -w .
	$(TF) fmt -recursive

validate: ## Run go vet and terraform validate
	go vet ./...
	$(TF) validate

test: ## Run all Go unit tests
	go test ./... -count=1

# ============================================================
# Demo — covers spec scenarios 1–4 and 7 inline
# ============================================================
demo: ## Send sample messages and print the DynamoDB result
	@command -v aws > /dev/null 2>&1 || \
		{ echo "ERROR: aws CLI not found. Install awscli."; exit 1; }

	$(eval CLIENTES_Q  := $(shell $(TF) output -raw clientes_queue_url  2>/dev/null))
	$(eval BLOQUEIOS_Q := $(shell $(TF) output -raw bloqueios_queue_url 2>/dev/null))
	$(eval CLIENTES_DLQ := $(shell $(TF) output -raw clientes_dlq_url   2>/dev/null))

	@test -n "$(CLIENTES_Q)" || { echo "ERROR: run 'make deploy' first"; exit 1; }

	@echo ""
	@echo "=== Scenario 1: New client ==="
	$(AWS_LOCAL) sqs send-message \
		--queue-url "$(CLIENTES_Q)" \
		--message-body '{"codigo_cliente":"$(DEMO_CLIENT)","numero_agencia":1,"numero_conta":100,"numero_dac":5,"data_atualizacao":"2024-01-15T10:00:00Z"}'

	@echo ""
	@echo "=== Scenario 2: Blocks before client exists (same GUID) ==="
	$(AWS_LOCAL) sqs send-message \
		--queue-url "$(BLOQUEIOS_Q)" \
		--message-body '{"codigo_cliente":"$(DEMO_CLIENT)","codigo_bloqueio_1":200,"valor_bloqueio_1":"50.00","data_atualizacao":"2024-01-15T10:30:00Z"}'

	@echo ""
	@echo "=== Scenario 3: Older client message — must be discarded ==="
	$(AWS_LOCAL) sqs send-message \
		--queue-url "$(CLIENTES_Q)" \
		--message-body '{"codigo_cliente":"$(DEMO_CLIENT)","numero_agencia":1,"numero_conta":100,"numero_dac":5,"data_atualizacao":"2024-01-10T08:00:00Z"}'

	@echo ""
	@echo "=== Scenario 4: Updated block list — replaces previous ==="
	$(AWS_LOCAL) sqs send-message \
		--queue-url "$(BLOQUEIOS_Q)" \
		--message-body '{"codigo_cliente":"$(DEMO_CLIENT)","codigo_bloqueio_1":100,"valor_bloqueio_1":"25.00","codigo_bloqueio_2":200,"valor_bloqueio_2":"75.50","data_atualizacao":"2024-01-16T09:00:00Z"}'

	@echo ""
	@echo "=== Scenario 7: Invalid message in batch — only it fails ==="
	$(AWS_LOCAL) sqs send-message \
		--queue-url "$(CLIENTES_Q)" \
		--message-body '{"codigo_cliente":"not-a-guid","numero_agencia":1}'

	@echo ""
	@echo "Waiting 10s for Lambda to process..."
	@sleep 10

	@echo ""
	@echo "=== DynamoDB record for $(DEMO_CLIENT) ==="
	$(AWS_LOCAL) dynamodb get-item \
		--table-name cadastro_geral \
		--key '{"codigo_cliente":{"S":"$(DEMO_CLIENT)"}}' \
		--output json

	@echo ""
	@echo "=== DLQ message count (invalid message should be here after 3 attempts) ==="
	$(AWS_LOCAL) sqs get-queue-attributes \
		--queue-url "$(CLIENTES_DLQ)" \
		--attribute-names ApproximateNumberOfMessages \
		--query 'Attributes.ApproximateNumberOfMessages' \
		--output text | xargs printf "Messages in DLQ: %s\n"

# ============================================================
# Logs
# ============================================================
logs: ## Stream logs from the emulator container
	docker compose logs -f

# ============================================================
# Help
# ============================================================
help: ## Show available targets
	@echo "Usage: make [target] [ARCH=amd64|arm64] [ENDPOINT=http://...] [REGION=...]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
