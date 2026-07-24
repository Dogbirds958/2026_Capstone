#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
INSTALL_DIR="${ONNXRUNTIME_ROOT:-$PROJECT_ROOT/third_party/onnxruntime}"
ONNXRUNTIME_VERSION="${ONNXRUNTIME_VERSION:-1.22.0}"

case "$(uname -s)-$(uname -m)" in
	Linux-x86_64)
		PLATFORM="linux-x64"
		;;
	Linux-aarch64|Linux-arm64)
		PLATFORM="linux-aarch64"
		;;
	*)
		echo "error: unsupported platform: $(uname -s)-$(uname -m)" >&2
		exit 1
		;;
esac

ARCHIVE="onnxruntime-$PLATFORM-$ONNXRUNTIME_VERSION.tgz"
DOWNLOAD_URL="https://github.com/microsoft/onnxruntime/releases/download/v$ONNXRUNTIME_VERSION/$ARCHIVE"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

if [ -f "$INSTALL_DIR/include/onnxruntime_c_api.h" ] && \
	[ -f "$INSTALL_DIR/include/onnxruntime_cxx_api.h" ] && \
	[ -f "$INSTALL_DIR/lib/libonnxruntime.so" ]; then
	echo "ONNX Runtime C++ already exists: $INSTALL_DIR"
	exit 0
fi

echo "downloading ONNX Runtime $ONNXRUNTIME_VERSION CPU package for $PLATFORM"
if command -v curl >/dev/null 2>&1; then
	curl --fail --location --retry 3 "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE"
elif command -v wget >/dev/null 2>&1; then
	wget -O "$TMP_DIR/$ARCHIVE" "$DOWNLOAD_URL"
else
	echo "error: curl or wget is required" >&2
	exit 1
fi

tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"
EXTRACTED_DIR="$TMP_DIR/onnxruntime-$PLATFORM-$ONNXRUNTIME_VERSION"

if [ ! -d "$EXTRACTED_DIR/include" ] || [ ! -f "$EXTRACTED_DIR/lib/libonnxruntime.so" ]; then
	echo "error: downloaded package does not contain the expected headers and library" >&2
	exit 1
fi

mkdir -p "$INSTALL_DIR/include" "$INSTALL_DIR/lib"
cp -R "$EXTRACTED_DIR/include/." "$INSTALL_DIR/include/"
cp -R "$EXTRACTED_DIR/lib/." "$INSTALL_DIR/lib/"

echo "ONNX Runtime C++ ready: $INSTALL_DIR"
echo "configure with: cmake -DONNXRUNTIME_ROOT=$INSTALL_DIR -S ai -B build/ai"
