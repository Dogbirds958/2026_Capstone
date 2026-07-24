#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VENV_DIR="$PROJECT_ROOT/.venv-onnx-export"
PYTHON_BIN="${PYTHON_BIN:-python3}"
TORCH_VERSION="${TORCH_VERSION:-2.13.0+cpu}"
PYTORCH_CPU_INDEX="${PYTORCH_CPU_INDEX:-https://download.pytorch.org/whl/cpu}"

if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
	echo "error: $PYTHON_BIN is required" >&2
	exit 1
fi

if [ ! -x "$VENV_DIR/bin/python" ]; then
	echo "creating Python virtual environment: $VENV_DIR"
	if ! "$PYTHON_BIN" -m venv "$VENV_DIR"; then
		echo "error: failed to create a virtual environment" >&2
		echo "install the Python venv package for your OS and run this script again" >&2
		exit 1
	fi
fi

echo "installing pinned model preparation dependencies"
"$VENV_DIR/bin/python" -m pip install --upgrade pip
"$VENV_DIR/bin/python" -m pip install \
	--index-url "$PYTORCH_CPU_INDEX" \
	"torch==$TORCH_VERSION"
"$VENV_DIR/bin/python" -m pip install \
	-r "$PROJECT_ROOT/ai/requirements.txt"

echo "generating and validating the ONNX model"
"$VENV_DIR/bin/python" "$PROJECT_ROOT/ai/prepare_model.py"

echo "ONNX model environment ready"
echo "activate it with: . $VENV_DIR/bin/activate"
