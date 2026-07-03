#!/bin/sh
set -eu

ROOTFS="${1:-busybox-rootfs}"
BUSYBOX_URL="${BUSYBOX_URL:-https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox}"
BUSYBOX_PATH="$ROOTFS/bin/busybox"

if [ -x "$BUSYBOX_PATH" ]; then
	echo "busybox already exists: $BUSYBOX_PATH"
	exit 0
fi

echo "busybox not found. preparing rootfs: $ROOTFS"

mkdir -p "$ROOTFS/bin" "$ROOTFS/proc" "$ROOTFS/dev" "$ROOTFS/tmp" "$ROOTFS/etc" "$ROOTFS/sys"

if command -v curl >/dev/null 2>&1; then
	curl -L "$BUSYBOX_URL" -o "$BUSYBOX_PATH"
elif command -v wget >/dev/null 2>&1; then
	wget -O "$BUSYBOX_PATH" "$BUSYBOX_URL"
else
	echo "curl or wget is required to download busybox" >&2
	exit 1
fi

chmod +x "$BUSYBOX_PATH"

for applet in sh ls cat ps mount umount echo pwd mkdir rmdir rm cp mv touch chmod chown sleep uname hostname id env; do
	ln -sf busybox "$ROOTFS/bin/$applet"
done

echo "busybox rootfs ready: $ROOTFS"
