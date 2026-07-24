#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNNER="${1:-$PROJECT_ROOT/build/ai/inference-runner}"
ROOTFS="${2:-$PROJECT_ROOT/busybox-rootfs}"

if [ ! -x "$RUNNER" ]; then
	echo "error: inference runner is not executable: $RUNNER" >&2
	exit 1
fi
if ! command -v ldd >/dev/null 2>&1; then
	echo "error: ldd is required" >&2
	exit 1
fi

REPORT=$(mktemp)
DEPENDENCIES=$(mktemp)
trap 'rm -f "$REPORT" "$DEPENDENCIES"' EXIT HUP INT TERM

if ! ldd "$RUNNER" >"$REPORT" 2>&1; then
	echo "error: ldd failed for $RUNNER" >&2
	cat "$REPORT" >&2
	exit 1
fi

if grep -q '=>[[:space:]]*not found' "$REPORT"; then
	echo "error: unresolved shared library dependency" >&2
	cat "$REPORT" >&2
	exit 1
fi

# Emit "requested library name<TAB>resolved host path". The second rule
# captures the ELF interpreter, whose ldd line does not contain "=>".
awk '
    $2 == "=>" && $3 ~ /^\// { print $1 "\t" $3 }
    $1 ~ /^\// { path = $1; sub(/^.*\//, "", path); print path "\t" $1 }
' "$REPORT" >"$DEPENDENCIES"

if [ ! -s "$DEPENDENCIES" ]; then
	echo "error: no file-backed shared dependencies found" >&2
	exit 1
fi

echo "required shared libraries:"
while IFS="$(printf '\t')" read -r library_name host_path; do
	printf '  %s -> %s\n' "$library_name" "$host_path"
done <"$DEPENDENCIES"

mkdir -p "$ROOTFS/lib" "$ROOTFS/lib64"

if [ ! -w "$ROOTFS/lib" ] || [ ! -w "$ROOTFS/lib64" ]; then
	echo "error: rootfs library directories are not writable: $ROOTFS" >&2
	echo "rerun this script with sufficient permission (for example, sudo)" >&2
	exit 1
fi

echo "copied shared libraries:"
onnxruntime_directory=""
while IFS="$(printf '\t')" read -r library_name host_path; do
	case "$host_path" in
		/lib64/*|/usr/lib64/*)
			destination_directory="$ROOTFS/lib64"
			;;
		*)
			destination_directory="$ROOTFS/lib"
			;;
	esac

	destination="$destination_directory/$library_name"
	cp -L "$host_path" "$destination"
	printf '  %s\n' "$destination"

	case "$library_name" in
		libonnxruntime.so*)
			onnxruntime_directory=$(dirname -- "$host_path")
			;;
	esac
done <"$DEPENDENCIES"

# Keep the unversioned linker name requested by the project layout in addition
# to the SONAME required by the executable at runtime.
if [ -n "$onnxruntime_directory" ] && [ -e "$onnxruntime_directory/libonnxruntime.so" ]; then
	destination="$ROOTFS/lib/libonnxruntime.so"
	cp -L "$onnxruntime_directory/libonnxruntime.so" "$destination"
	printf '  %s\n' "$destination"
else
	echo "error: libonnxruntime.so was not found in the resolved dependency directory" >&2
	exit 1
fi
