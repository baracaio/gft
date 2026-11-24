#!/usr/bin/env bash
# run-scenario.sh — load a sample file and run it against the local stack.
# Usage:
#   bash scripts/run-scenario.sh          # interactive prompt
#   bash scripts/run-scenario.sh 3        # run scenario 3 directly
#
# Override defaults:
#   ENDPOINT=http://localhost:4566  REGION=us-east-1  bash scripts/run-scenario.sh

set -euo pipefail

ENDPOINT="${ENDPOINT:-http://localhost:4566}"
REGION="${REGION:-us-east-1}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SAMPLES_DIR="$PROJECT_ROOT/samples"

AWS="aws --endpoint-url=$ENDPOINT --region=$REGION"
TF="terraform -chdir=$PROJECT_ROOT/infra"

# ── Helpers ───────────────────────────────────────────────────────────────────
hr() { echo "────────────────────────────────────────────────────────────────────────"; }

py() { python3 -c "$1"; }

json_get() {
  # json_get <file> <python-expression-on-d>
  py "import json; d=json.load(open('$1')); print($2)"
}

# ── Scenario selection ────────────────────────────────────────────────────────
if [ -n "${1:-}" ]; then
  SCENARIO="$1"
else
  echo ""
  echo "Available scenarios:"
  echo ""
  for f in "$SAMPLES_DIR"/[0-9]*.json; do
    py "
import json
d = json.load(open('$f'))
print(f'  {d[\"scenario\"]:2}.  {d[\"title\"]}')
"
  done
  echo ""
  printf "Scenario number: "
  read -r SCENARIO
fi

# ── Locate sample file ────────────────────────────────────────────────────────
SAMPLE=$(ls "$SAMPLES_DIR"/$(printf "%02d" "$SCENARIO")-*.json 2>/dev/null | head -1)
if [ -z "$SAMPLE" ]; then
  echo "ERROR: no sample file for scenario $SCENARIO (expected $SAMPLES_DIR/$(printf '%02d' "$SCENARIO")-*.json)" >&2
  exit 1
fi

# ── Parse metadata ────────────────────────────────────────────────────────────
TITLE=$(json_get "$SAMPLE" "d['title']")
DESCRIPTION=$(json_get "$SAMPLE" "d['description']")
STEPS=$(json_get "$SAMPLE" "len(d['steps'])")
OUTCOME=$(json_get "$SAMPLE" "d['expect']['outcome']")
CLIENT_GUID=$(json_get "$SAMPLE" "d['steps'][0]['body'].get('codigo_cliente', '')")

SHOW_DLQ=$(json_get "$SAMPLE"   "'yes' if 'dlq' in d.get('expect', {}) else 'no'")
SHOW_FAILURES=$(json_get "$SAMPLE" "'yes' if int(d['scenario']) == 9 else 'no'")

# ── Resolve queue URLs ────────────────────────────────────────────────────────
CLIENTES_Q=$($TF output -raw clientes_queue_url  2>/dev/null)
BLOQUEIOS_Q=$($TF output -raw bloqueios_queue_url 2>/dev/null)
CLIENTES_DLQ=$($TF output -raw clientes_dlq_url  2>/dev/null)
BLOQUEIOS_DLQ=$($TF output -raw bloqueios_dlq_url 2>/dev/null)

queue_url() {
  case "$1" in
    clientes)      echo "$CLIENTES_Q"  ;;
    bloqueios)     echo "$BLOQUEIOS_Q" ;;
    *) echo "ERROR: unknown queue name '$1'" >&2; exit 1 ;;
  esac
}

# ── Print header ──────────────────────────────────────────────────────────────
echo ""
hr
echo "  Scenario $SCENARIO — $TITLE"
hr
echo ""
echo "$DESCRIPTION"
echo ""
echo "Expected: $OUTCOME"
echo ""

# ── Send messages ─────────────────────────────────────────────────────────────
send_step() {
  local idx=$1
  local queue body url label

  queue=$(json_get "$SAMPLE" "d['steps'][$idx]['queue']")
  body=$(py "import json; d=json.load(open('$SAMPLE')); print(json.dumps(d['steps'][$idx]['body']))")
  url=$(queue_url "$queue")
  label=$(py "import json; d=json.load(open('$SAMPLE')); print(d['steps'][$idx]['body'].get('codigo_cliente','?'))")

  echo "  → [$queue]  $label"
  MSG_ID=$($AWS sqs send-message \
    --queue-url "$url" \
    --message-body "$body" \
    --query 'MessageId' --output text)
  echo "    MessageId: $MSG_ID"
}

echo "Sending messages..."
echo ""

if [ "$SCENARIO" -eq 6 ]; then
  # Scenario 6 requires concurrent delivery to provoke a version conflict.
  send_step 0 &
  send_step 1 &
  wait
else
  for i in $(seq 0 $((STEPS - 1))); do
    send_step "$i"
  done
fi

echo ""
echo "Waiting 10 seconds for Lambda to process..."
sleep 10

# ── Show results ──────────────────────────────────────────────────────────────
echo ""
hr
echo "  Results"
hr

echo ""
echo "── cadastro_geral  (key: $CLIENT_GUID) ──"
DYNAMO_OUT=$($AWS dynamodb get-item \
  --table-name cadastro_geral \
  --key "{\"codigo_cliente\":{\"S\":\"$CLIENT_GUID\"}}" \
  --output json 2>&1)

if echo "$DYNAMO_OUT" | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get('Item') else 1)" 2>/dev/null; then
  echo "$DYNAMO_OUT" | python3 -c "
import json, sys
item = json.load(sys.stdin)['Item']

def val(v):
    if 'S' in v: return v['S']
    if 'N' in v: return v['N']
    if 'L' in v: return [val(x) for x in v['L']]
    if 'M' in v: return {k: val(u) for k,u in v['M'].items()}
    return v

for k, v in sorted(item.items()):
    print(f'  {k}: {val(v)}')
"
else
  echo "  (no record — item does not exist)"
fi

if [ "$SHOW_DLQ" = "yes" ]; then
  echo ""
  echo "── Dead-letter queues ──"
  CLIENTES_DLQ_N=$($AWS sqs get-queue-attributes \
    --queue-url "$CLIENTES_DLQ" \
    --attribute-names ApproximateNumberOfMessages \
    --query 'Attributes.ApproximateNumberOfMessages' --output text)
  BLOQUEIOS_DLQ_N=$($AWS sqs get-queue-attributes \
    --queue-url "$BLOQUEIOS_DLQ" \
    --attribute-names ApproximateNumberOfMessages \
    --query 'Attributes.ApproximateNumberOfMessages' --output text)

  echo "  clientes DLQ:  $CLIENTES_DLQ_N message(s)"
  echo "  bloqueios DLQ: $BLOQUEIOS_DLQ_N message(s)"
  echo ""
  echo "  Note: the invalid message needs 3 delivery attempts to reach the DLQ."
  echo "  With LAMBDA_TIMEOUT=5 (visibility=30s) that takes ~60-90 seconds."
  echo "  Re-run this check later: bash scripts/check-dlq.sh"
fi

if [ "$SHOW_FAILURES" = "yes" ]; then
  echo ""
  echo "── falhas_processamento ──"
  $AWS dynamodb scan --table-name falhas_processamento --output json | python3 -c "
import json, sys
result = json.load(sys.stdin)
items = result.get('Items', [])
if not items:
    print('  (empty — the message has not exhausted its retries yet)')
else:
    for item in items:
        def val(v):
            if 'S' in v: return v['S']
            if 'N' in v: return v['N']
            return str(v)
        print('')
        for k, v in sorted(item.items()):
            print(f'  {k}: {val(v)}')
"
fi

echo ""
hr
