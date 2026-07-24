#!/bin/sh
set -eu
. "$(dirname -- "$0")/test_common.sh"
require_root
create_test_workspace

write_test_config "max 100000" "1073741824" "64" \
	/bin/inference-runner --model /models/current/model.onnx \
	--input /models/current/input.bin

python3 - "$TEST_CONFIG" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, encoding="utf-8") as source:
    config = json.load(source)
config["model"]["host_path"] = config["model"]["host_path"] + "-missing"
with open(path, "w", encoding="utf-8") as output:
    json.dump(config, output)
PY
set +e
MISSING_OUTPUT=$(MINI_CONTAINER_CONFIG="$TEST_CONFIG" "$RUNTIME" 2>&1)
MISSING_EXIT=$?
set -e
if [ "$MISSING_EXIT" -eq 0 ] || ! printf '%s\n' "$MISSING_OUTPUT" | grep -q "model.host_path"; then
	echo "error: missing model path was not rejected" >&2
	exit 1
fi
if printf '%s\n' "$MISSING_OUTPUT" | grep -q "child pid:"; then
	echo "error: container started for missing model path" >&2
	exit 1
fi

write_test_config "max 100000" "1073741824" "64" \
	/bin/inference-runner --model /models/current/model.onnx \
	--input /models/current/input.bin
python3 - "$TEST_MODEL/manifest.json" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, encoding="utf-8") as source:
    manifest = json.load(source)
manifest["sha256"][manifest["model_file"]] = "0" * 64
with open(path, "w", encoding="utf-8") as output:
    json.dump(manifest, output)
PY
set +e
HASH_OUTPUT=$(MINI_CONTAINER_CONFIG="$TEST_CONFIG" "$RUNTIME" 2>&1)
HASH_EXIT=$?
set -e
if [ "$HASH_EXIT" -eq 0 ] || ! printf '%s\n' "$HASH_OUTPUT" | grep -q "sha256 mismatch"; then
	echo "error: corrupted model hash was not rejected" >&2
	exit 1
fi
if printf '%s\n' "$HASH_OUTPUT" | grep -q "child pid:"; then
	echo "error: container started for corrupted model hash" >&2
	exit 1
fi

echo "missing_model_test=passed"
echo "corrupted_model_hash_test=passed"
