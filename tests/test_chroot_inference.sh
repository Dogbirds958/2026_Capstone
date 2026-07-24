#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ROOTFS="$PROJECT_ROOT/busybox-rootfs"
MODEL="$PROJECT_ROOT/model-store/simple-classifier/1.0.0"
TARGET="$ROOTFS/models/current"

if [ "${CHROOT_TEST_INNER:-0}" != "1" ]; then
	if [ "$(id -u)" -ne 0 ]; then
		echo "error: run this test with sudo" >&2
		exit 1
	fi
	exec env CHROOT_TEST_INNER=1 unshare --mount "$0"
fi

mount --make-rprivate /
mkdir -p "$TARGET"
mount --bind "$MODEL" "$TARGET"
mount -o remount,bind,ro "$TARGET"

OUTPUT=$(chroot "$ROOTFS" /bin/inference-runner \
	--model /models/current/model.onnx \
	--input /models/current/input.bin \
	--repeat 2)
printf '%s\n' "$OUTPUT"
printf '%s\n' "$OUTPUT" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["inference"]["repeat"] == 2
assert isinstance(result["result"]["class"], int)
'
echo "chroot_inference_test=passed"
