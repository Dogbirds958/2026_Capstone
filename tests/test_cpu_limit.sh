#!/bin/sh
set -eu
. "$(dirname -- "$0")/test_common.sh"
require_root
create_test_workspace

LIMIT="25000 100000"
write_test_config "$LIMIT" "1073741824" "64" \
	/bin/inference-runner --model /models/current/model.onnx \
	--input /models/current/input.bin --repeat 10000
OUTPUT=$(MINI_CONTAINER_CONFIG="$TEST_CONFIG" "$RUNTIME")
printf '%s\n' "$OUTPUT" | python3 -c '
import json, sys
result = json.load(sys.stdin)
cgroup = result["cgroup"]
assert result["status"] == "success"
assert cgroup["cpu_max"] == "25000 100000"
assert cgroup["cpu_stat"]["usage_usec"] > 0
assert cgroup["cpu_stat"]["nr_periods"] > 0
'
echo "cpu_limit_test=passed"
