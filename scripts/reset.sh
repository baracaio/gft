#!/usr/bin/env bash
# reset.sh — wipe all local state so the demo can be run from scratch.
# Purges the four SQS queues, removes the demo client from cadastro_geral,
# and deletes every row from falhas_processamento.
#
# Override defaults with environment variables:
#   ENDPOINT=http://localhost:4566  REGION=us-east-1  make reset

set -euo pipefail

ENDPOINT="${ENDPOINT:-http://localhost:4566}"
REGION="${REGION:-us-east-1}"
DEMO_CLIENT="${DEMO_CLIENT:-550e8400-e29b-41d4-a716-446655440000}"

AWS="aws --endpoint-url=$ENDPOINT --region=$REGION"
TF="terraform -chdir=infra"

# ── Resolve queue URLs from Terraform output ─────────────────────────────────
CLIENTES_Q=$($TF output -raw clientes_queue_url  2>/dev/null || true)
BLOQUEIOS_Q=$($TF output -raw bloqueios_queue_url 2>/dev/null || true)
CLIENTES_DLQ=$($TF output -raw clientes_dlq_url  2>/dev/null || true)
BLOQUEIOS_DLQ=$($TF output -raw bloqueios_dlq_url 2>/dev/null || true)

if [ -z "$CLIENTES_Q" ]; then
  echo "ERROR: could not read Terraform outputs. Run 'make deploy' first." >&2
  exit 1
fi

# ── SQS queues ────────────────────────────────────────────────────────────────
echo "Purging SQS queues..."
$AWS sqs purge-queue --queue-url "$CLIENTES_Q"
$AWS sqs purge-queue --queue-url "$BLOQUEIOS_Q"
$AWS sqs purge-queue --queue-url "$CLIENTES_DLQ"
$AWS sqs purge-queue --queue-url "$BLOQUEIOS_DLQ"
echo "  all four queues purged"

# ── DynamoDB: cadastro_geral (demo client only) ───────────────────────────────
echo "Deleting demo client from cadastro_geral..."
$AWS dynamodb delete-item \
  --table-name cadastro_geral \
  --key "{\"codigo_cliente\":{\"S\":\"$DEMO_CLIENT\"}}" 2>/dev/null || true
echo "  done"

# ── DynamoDB: falhas_processamento (all rows) ─────────────────────────────────
echo "Clearing falhas_processamento..."
IDS=$($AWS dynamodb scan \
  --table-name falhas_processamento \
  --projection-expression "id_mensagem" \
  --query "Items[].id_mensagem.S" \
  --output text 2>/dev/null || true)

COUNT=0
for id in $IDS; do
  [ -z "$id" ] && continue
  $AWS dynamodb delete-item \
    --table-name falhas_processamento \
    --key "{\"id_mensagem\":{\"S\":\"$id\"}}" 2>/dev/null || true
  COUNT=$((COUNT + 1))
done
echo "  $COUNT failure record(s) deleted"

echo ""
echo "Reset complete. Ready to run: make demo"
