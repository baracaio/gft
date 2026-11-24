#!/usr/bin/env bash
# check-dlq.sh — show current state of all queues and the failure table.
# Useful after running a failure scenario (7, 8, 9) to confirm that invalid
# messages reached the DLQ and that the failure record was written.
#
# DLQ messages only appear after 3 delivery attempts. With the default
# 180-second visibility timeout that takes roughly 6-9 minutes.
#
# Usage:
#   bash scripts/check-dlq.sh
#   ENDPOINT=http://localhost:4566 bash scripts/check-dlq.sh

set -euo pipefail

ENDPOINT="${ENDPOINT:-http://localhost:4566}"
REGION="${REGION:-us-east-1}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

AWS="aws --endpoint-url=$ENDPOINT --region=$REGION"
TF="terraform -chdir=$PROJECT_ROOT/infra"

hr()  { echo "────────────────────────────────────────────────────────────────────────"; }
HR()  { echo "════════════════════════════════════════════════════════════════════════"; }

# ── Resolve URLs ──────────────────────────────────────────────────────────────
CLIENTES_Q=$($TF output -raw clientes_queue_url   2>/dev/null)
BLOQUEIOS_Q=$($TF output -raw bloqueios_queue_url  2>/dev/null)
CLIENTES_DLQ=$($TF output -raw clientes_dlq_url   2>/dev/null)
BLOQUEIOS_DLQ=$($TF output -raw bloqueios_dlq_url  2>/dev/null)

# ── Queue depth helper ────────────────────────────────────────────────────────
queue_depths() {
  local url="$1"
  local attrs
  attrs=$($AWS sqs get-queue-attributes \
    --queue-url "$url" \
    --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible \
    --output json 2>/dev/null)
  local visible in_flight
  visible=$(echo "$attrs"   | python3 -c "import json,sys; print(json.load(sys.stdin)['Attributes']['ApproximateNumberOfMessages'])")
  in_flight=$(echo "$attrs" | python3 -c "import json,sys; print(json.load(sys.stdin)['Attributes']['ApproximateNumberOfMessagesNotVisible'])")
  echo "$visible $in_flight"
}

# ── Peek into a DLQ (non-destructive, visibility-timeout 0) ───────────────────
peek_dlq() {
  local url="$1" label="$2"
  local raw
  raw=$($AWS sqs receive-message \
    --queue-url "$url" \
    --max-number-of-messages 10 \
    --visibility-timeout 0 \
    --output json 2>/dev/null)
  [ -z "$raw" ] && raw='{}'

  local count
  count=$(echo "$raw" | python3 -c "
import json, sys
msgs = json.load(sys.stdin).get('Messages', [])
print(len(msgs))
")

  if [ "$count" -eq 0 ]; then
    echo "  (empty)"
    return
  fi

  echo "$raw" | python3 -c "
import json, sys
msgs = json.load(sys.stdin).get('Messages', [])
for i, m in enumerate(msgs, 1):
    body_raw = m.get('Body', '')
    try:
        body = json.loads(body_raw)
        client = body.get('codigo_cliente', '—')
        ts     = body.get('data_atualizacao', '—')
        summary = f'cliente={client}  ts={ts}'
    except Exception:
        summary = body_raw[:80]
    print(f'  [{i}] MessageId: {m[\"MessageId\"]}')
    print(f'       {summary}')
"
}

# ── Header ────────────────────────────────────────────────────────────────────
echo ""
HR
echo "  DLQ & Queue Status"
HR

# ── Queue depth table ─────────────────────────────────────────────────────────
echo ""
echo "── SQS Queue Depths ──"
echo ""
printf "  %-30s  %8s  %10s\n" "Queue" "Visible" "In-flight"
printf "  %-30s  %8s  %10s\n" "─────────────────────────────" "───────" "─────────"

for label_url in \
    "clientes|$CLIENTES_Q" \
    "clientes DLQ|$CLIENTES_DLQ" \
    "bloqueios|$BLOQUEIOS_Q" \
    "bloqueios DLQ|$BLOQUEIOS_DLQ"
do
  label="${label_url%%|*}"
  url="${label_url##*|}"
  read -r vis inf <<< "$(queue_depths "$url")"
  printf "  %-30s  %8s  %10s\n" "$label" "$vis" "$inf"
done

echo ""
echo "  In-flight = messages currently hidden while Lambda processes or retries them."

# ── DLQ message bodies ────────────────────────────────────────────────────────
echo ""
hr
echo "── DLQ Messages ──"

echo ""
echo "  clientes DLQ:"
peek_dlq "$CLIENTES_DLQ" "clientes DLQ"

echo ""
echo "  bloqueios DLQ:"
peek_dlq "$BLOQUEIOS_DLQ" "bloqueios DLQ"

# ── Failure table ─────────────────────────────────────────────────────────────
echo ""
hr
echo "── falhas_processamento ──"
echo ""

FAILURES=$($AWS dynamodb scan --table-name falhas_processamento --output json 2>/dev/null)
COUNT=$(echo "$FAILURES" | python3 -c "import json,sys; print(json.load(sys.stdin)['Count'])")

if [ "$COUNT" -eq 0 ]; then
  echo "  (empty — no message has exhausted all retries yet)"
else
  echo "  $COUNT record(s) found:"
  echo "$FAILURES" | python3 -c "
import json, sys

def val(v):
    if 'S' in v: return v['S']
    if 'N' in v: return v['N']
    return str(v)

items = json.load(sys.stdin)['Items']
ORDER = ['id_mensagem','dominio','fila','codigo_cliente','tipo_erro','tentativas','data_falha','mensagem_erro','corpo_original']

for item in items:
    print('')
    shown = set()
    for k in ORDER:
        if k in item:
            v = val(item[k])
            if k == 'corpo_original' and len(v) > 80:
                v = v[:77] + '...'
            print(f'  {k}: {v}')
            shown.add(k)
    for k in sorted(item):
        if k not in shown:
            print(f'  {k}: {val(item[k])}')
"
fi

echo ""
HR
