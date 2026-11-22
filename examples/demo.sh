#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

CAPTURE=/tmp/apishadow_capture.jsonl
PRIMARY=/tmp/apishadow_primary.jsonl
SHADOW=/tmp/apishadow_shadow.jsonl
REPORT=/tmp/apishadow_report.json
rm -f "$CAPTURE" "$PRIMARY" "$SHADOW" "$REPORT"

ROLE=primary PORT=9101 python3 examples/echo_server.py >/tmp/apishadow_primary.log 2>&1 &
P1=$!
ROLE=shadow PORT=9102 python3 examples/echo_server.py >/tmp/apishadow_shadow.log 2>&1 &
P2=$!
P3=""
cleanup() {
  kill "$P1" "$P2" ${P3:+"$P3"} 2>/dev/null || true
}
trap cleanup EXIT

for port in 9101 9102; do
  curl -s --retry 20 --retry-connrefused --retry-delay 1 "http://127.0.0.1:$port/healthz" -o /dev/null || true
done

TMP_CONFIG=$(mktemp)
sed "s|capture_file=.*|capture_file=$CAPTURE|" config/apishadow.conf > "$TMP_CONFIG"
(cd go && go build -o /tmp/apishadow_proxy ./cmd/proxy)
/tmp/apishadow_proxy -config "$TMP_CONFIG" >/tmp/apishadow_proxy.log 2>&1 &
P3=$!

curl -s --retry 20 --retry-connrefused --retry-delay 1 http://127.0.0.1:8080/healthz >/dev/null
curl -s http://127.0.0.1:8080/v1/users -o /dev/null
curl -s -X POST http://127.0.0.1:8080/v1/items -d '{"q":1}' -o /dev/null
curl -s http://127.0.0.1:8080/v1/error -o /dev/null
sleep 1

python3 - "$CAPTURE" "$PRIMARY" "$SHADOW" <<'PY'
import json, sys
source, primary_path, shadow_path = sys.argv[1:4]
primary, shadow = [], []
for line in open(source, encoding="utf-8"):
    if not line.strip():
        continue
    record = json.loads(line)
    (primary if record["role"] == "primary" else shadow).append(record)
for path, rows in ((primary_path, primary), (shadow_path, shadow)):
    with open(path, "w", encoding="utf-8") as out:
        for row in rows:
            out.write(json.dumps(row, ensure_ascii=False) + "\n")
print(f"captured primary={len(primary)} shadow={len(shadow)}")
PY

PYTHONPATH="$ROOT/python" python3 -m apishadow \
  --primary "$PRIMARY" --shadow "$SHADOW" --out "$REPORT"

PYTHONPATH="$ROOT/python" python3 -m apishadow.clickhouse \
  --report "$REPORT" --out /tmp/apishadow_diffs.ndjson

echo "report: $REPORT"
