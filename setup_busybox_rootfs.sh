#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOTFS="${1:-$PROJECT_ROOT/busybox-rootfs}"
case "$ROOTFS" in
	/*) ;;
	*) ROOTFS="$PROJECT_ROOT/$ROOTFS" ;;
esac

BUSYBOX_URL="${BUSYBOX_URL:-https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox}"
BUSYBOX_PATH="$ROOTFS/bin/busybox"
INFERENCE_RUNNER="$PROJECT_ROOT/build/ai/inference-runner"
DEPENDENCY_SCRIPT="$PROJECT_ROOT/scripts/copy_runtime_dependencies.sh"

setup_directories() {
	echo "setting up rootfs directories"
	mkdir -p \
		"$ROOTFS/bin" \
		"$ROOTFS/lib" \
		"$ROOTFS/lib64" \
		"$ROOTFS/proc" \
		"$ROOTFS/dev" \
		"$ROOTFS/sys" \
		"$ROOTFS/tmp" \
		"$ROOTFS/run" \
		"$ROOTFS/models/current" \
		"$ROOTFS/app"
}

setup_busybox() {
	if [ -x "$BUSYBOX_PATH" ]; then
		echo "busybox already exists: $BUSYBOX_PATH"
		return
	fi

	echo "downloading busybox: $BUSYBOX_URL"
	if command -v curl >/dev/null 2>&1; then
		curl --fail --location "$BUSYBOX_URL" -o "$BUSYBOX_PATH"
	elif command -v wget >/dev/null 2>&1; then
		wget -O "$BUSYBOX_PATH" "$BUSYBOX_URL"
	else
		echo "error: curl or wget is required to download busybox" >&2
		exit 1
	fi
	chmod 0755 "$BUSYBOX_PATH"
}

setup_busybox_applets() {
	echo "updating busybox applets"
	for applet in sh ls cat ps mount umount echo pwd mkdir rmdir rm cp mv touch chmod chown sleep uname hostname id env; do
		ln -sf busybox "$ROOTFS/bin/$applet"
	done
}

setup_inference_runner() {
	if [ ! -x "$INFERENCE_RUNNER" ]; then
		echo "error: inference runner not found: $INFERENCE_RUNNER" >&2
		echo "build it with: cmake --build $PROJECT_ROOT/build/ai -j" >&2
		exit 1
	fi
	echo "updating inference runner"
	cp "$INFERENCE_RUNNER" "$ROOTFS/bin/inference-runner"
	chmod 0755 "$ROOTFS/bin/inference-runner"
}

setup_runtime_libraries() {
	if [ ! -x "$DEPENDENCY_SCRIPT" ]; then
		echo "error: dependency copy script not found: $DEPENDENCY_SCRIPT" >&2
		exit 1
	fi
	echo "updating ONNX Runtime and required shared libraries"
	"$DEPENDENCY_SCRIPT" "$INFERENCE_RUNNER" "$ROOTFS"
}

setup_model_directory() {
	echo "ensuring model directory exists: $ROOTFS/models/current"
	mkdir -p "$ROOTFS/models/current"
	# Models are supplied by the runtime's read-only bind mount, not the rootfs.
	rm -f \
		"$ROOTFS/models/current/model.onnx" \
		"$ROOTFS/models/current/input.bin" \
		"$ROOTFS/models/current/labels.json" \
		"$ROOTFS/models/current/manifest.json"
}

setup_directories
setup_busybox
setup_busybox_applets
setup_inference_runner
setup_runtime_libraries
setup_model_directory

echo "busybox inference rootfs ready: $ROOTFS"
