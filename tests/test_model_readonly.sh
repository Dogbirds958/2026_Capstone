#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME="$PROJECT_ROOT/mini-container"

if [ "$(id -u)" -ne 0 ]; then
	echo "error: run this test with sudo because the runtime needs root privileges" >&2
	exit 1
fi
if [ ! -x "$RUNTIME" ]; then
	echo "error: runtime executable not found: $RUNTIME" >&2
	exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "error: python3 is required to create the temporary test config" >&2
	exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM
TEST_CONFIG="$TMP_DIR/config.json"
TEST_MODEL="$TMP_DIR/model-store/simple-classifier/1.0.0"
mkdir -p "$TEST_MODEL"
cp -R "$PROJECT_ROOT/model-store/simple-classifier/1.0.0/." "$TEST_MODEL/"

python3 - "$PROJECT_ROOT/config.json" "$TEST_CONFIG" "$PROJECT_ROOT" "$TEST_MODEL" <<'PY'
import json
import os
import sys

source_path, output_path, project_root, test_model = sys.argv[1:]
with open(source_path, encoding="utf-8") as source:
    config = json.load(source)

config["rootfs"] = os.path.realpath(os.path.join(project_root, config["rootfs"]))
config["model"]["host_path"] = os.path.realpath(test_model)
config["model"]["read_only"] = True
config["container_id"] = config["container_id"] + "-readonly-test"
config["command"] = [
    "/bin/sh",
    "-c",
    """
set -u

if /bin/busybox head -c 1 /models/current/model.onnx >/dev/null; then
    echo model_read=ok
else
    echo model_read=failed
    exit 1
fi

if echo test 2>/dev/null > /models/current/model.onnx; then
    echo model_modify=unexpected_success
    exit 1
else
    echo model_modify=blocked
fi

if touch /models/current/new-file 2>/dev/null; then
    echo model_create=unexpected_success
    exit 1
else
    echo model_create=blocked
fi

if /bin/busybox timeout -k 1 2 /bin/rm -f /models/current/model.onnx 2>/dev/null; then
    echo model_delete=unexpected_success
    exit 1
else
    echo model_delete=blocked
fi

if touch /tmp/test-file && test -f /tmp/test-file; then
    echo tmp_write=ok
    rm -f /tmp/test-file
else
    echo tmp_write=failed
    exit 1
fi
""",
]

with open(output_path, "w", encoding="utf-8") as output:
    json.dump(config, output, indent=2)
    output.write("\n")
PY

echo "running model isolation checks (the delete check is limited to 2 seconds)"
if ! OUTPUT=$(MINI_CONTAINER_CONFIG="$TEST_CONFIG" MINI_CONTAINER_RAW_OUTPUT=1 "$RUNTIME" 2>&1); then
	printf '%s\n' "$OUTPUT" >&2
	echo "error: read-only model test runtime failed" >&2
	exit 1
fi
printf '%s\n' "$OUTPUT"

for expected in \
	model_read=ok \
	model_modify=blocked \
	model_create=blocked \
	model_delete=blocked \
	tmp_write=ok
do
	if ! printf '%s\n' "$OUTPUT" | grep -q "^$expected$"; then
		echo "error: missing expected result: $expected" >&2
		exit 1
	fi
done

echo "model_readonly_test=passed"
