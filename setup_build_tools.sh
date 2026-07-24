#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VENV_DIR="$PROJECT_ROOT/.venv-build-tools"
PYTHON_BIN="${PYTHON_BIN:-python3}"
CMAKE_VERSION="${CMAKE_VERSION:-3.31.6}"

if ! command -v c++ >/dev/null 2>&1; then
	echo "error: a C++ compiler is required" >&2
	echo "Ubuntu/Debian: sudo apt-get install build-essential" >&2
	echo "Fedora: sudo dnf install gcc-c++ make" >&2
	echo "Arch Linux: sudo pacman -S base-devel" >&2
	exit 1
fi

if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
	echo "error: $PYTHON_BIN is required to install the pinned CMake tool" >&2
	exit 1
fi

if [ ! -x "$VENV_DIR/bin/python" ]; then
	"$PYTHON_BIN" -m venv "$VENV_DIR"
fi

"$VENV_DIR/bin/python" -m pip install --upgrade pip
"$VENV_DIR/bin/python" -m pip install "cmake==$CMAKE_VERSION"

"$VENV_DIR/bin/cmake" --version | sed -n '1p'
c++ --version | sed -n '1p'
echo "activate build tools with: . $VENV_DIR/bin/activate"
