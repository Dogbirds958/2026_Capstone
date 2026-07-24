#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNNER="$PROJECT_ROOT/build/ai/inference-runner"
MODEL="$PROJECT_ROOT/model-store/simple-classifier/1.0.0"

OUTPUT=$(
	"$RUNNER" --model "$MODEL/model.onnx" --input "$MODEL/input.bin" --repeat 2
)
printf '%s\n' "$OUTPUT"
printf '%s\n' "$OUTPUT" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["inference"]["repeat"] == 2
assert len(result["inference"]["inference_runs_ms"]) == 2
assert isinstance(result["result"]["class"], int)
'
echo "host_inference_test=passed"
