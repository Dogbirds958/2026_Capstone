#!/bin/sh

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME="$PROJECT_ROOT/mini-container"

require_root() {
	if [ "$(id -u)" -ne 0 ]; then
		echo "error: run this test with sudo" >&2
		exit 1
	fi
}

create_test_workspace() {
	TEST_TMP_DIR=$(mktemp -d)
	trap 'rm -rf "$TEST_TMP_DIR"' EXIT HUP INT TERM
	TEST_MODEL="$TEST_TMP_DIR/model-store/simple-classifier/1.0.0"
	TEST_CONFIG="$TEST_TMP_DIR/config.json"
	mkdir -p "$TEST_MODEL"
	cp -R "$PROJECT_ROOT/model-store/simple-classifier/1.0.0/." "$TEST_MODEL/"
}

write_test_config() {
	cpu_max=$1
	memory_max=$2
	pids_max=$3
	shift 3
	python3 - "$PROJECT_ROOT/config.json" "$TEST_CONFIG" "$PROJECT_ROOT" "$TEST_MODEL" \
		"$cpu_max" "$memory_max" "$pids_max" "$@" <<'PY'
import json
import os
import sys

source, output, project_root, model, cpu_max, memory_max, pids_max, *command = sys.argv[1:]
with open(source, encoding="utf-8") as config_file:
    config = json.load(config_file)
config["rootfs"] = os.path.realpath(os.path.join(project_root, config["rootfs"]))
config["model"]["host_path"] = os.path.realpath(model)
config["container_id"] = f"automated-test-{os.getpid()}"
config["cpu_max"] = cpu_max
config["memory_max"] = memory_max
config["pids_max"] = pids_max
config["command"] = command
with open(output, "w", encoding="utf-8") as config_file:
    json.dump(config, config_file, indent=2)
    config_file.write("\n")
PY
}
