#!/bin/sh
set -eu
. "$(dirname -- "$0")/test_common.sh"
require_root
create_test_workspace

write_test_config "max 100000" "1073741824" "64" \
	/bin/inference-runner --model /models/current/model.onnx \
	--input /models/current/input.bin --repeat 2
OUTPUT=$(MINI_CONTAINER_CONFIG="$TEST_CONFIG" "$RUNTIME")
printf '%s\n' "$OUTPUT"
printf '%s\n' "$OUTPUT" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["status"] == "success"
assert result["inference"]["repeat"] == 2
assert isinstance(result["result"]["class"], int)
'

write_test_config "max 100000" "1073741824" "64" /bin/sh -c "exit 7"
set +e
MINI_CONTAINER_CONFIG="$TEST_CONFIG" MINI_CONTAINER_RAW_OUTPUT=1 "$RUNTIME" >/dev/null
EXIT_CODE=$?
set -e
if [ "$EXIT_CODE" -ne 7 ]; then
	echo "error: child exit code 7 became $EXIT_CODE" >&2
	exit 1
fi
echo "runtime_inference_test=passed"
echo "exit_code_propagation_test=passed"
