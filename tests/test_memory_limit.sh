#!/bin/sh
set -eu
. "$(dirname -- "$0")/test_common.sh"
require_root
create_test_workspace

LIMIT=134217728
write_test_config "max 100000" "$LIMIT" "64" \
	/bin/inference-runner --model /models/current/model.onnx \
	--input /models/current/input.bin
OUTPUT=$(MINI_CONTAINER_CONFIG="$TEST_CONFIG" "$RUNTIME")
printf '%s\n' "$OUTPUT"
printf '%s\n' "$OUTPUT" | python3 -c '
import json, sys
result = json.load(sys.stdin)
cgroup = result["cgroup"]
limit = 134217728
assert result["status"] == "success"
assert cgroup["memory_max"] == str(limit)
assert 0 < cgroup["memory_peak"] <= limit
assert cgroup["memory_events"].get("oom", 0) == 0
'
echo "memory_limit_test=passed"
