#!/bin/sh
set -eu

PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME="$PROJECT_ROOT/mini-container"
OUTPUT_FILE="${1:-$PROJECT_ROOT/build/cgroup-limit-results.json}"

if [ "$(id -u)" -ne 0 ]; then
	echo "error: run this test with sudo because cgroup setup requires root privileges" >&2
	exit 1
fi
if [ ! -x "$RUNTIME" ]; then
	echo "error: runtime executable not found: $RUNTIME" >&2
	exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "error: python3 is required to orchestrate the test matrix" >&2
	exit 1
fi

mkdir -p "$(dirname -- "$OUTPUT_FILE")"

python3 - "$PROJECT_ROOT" "$RUNTIME" "$OUTPUT_FILE" <<'PY'
import json
import os
import shutil
import subprocess
import sys
import tempfile

project_root, runtime, output_file = sys.argv[1:]
with open(os.path.join(project_root, "config.json"), encoding="utf-8") as source:
    base_config = json.load(source)

base_config["rootfs"] = os.path.realpath(
    os.path.join(project_root, base_config["rootfs"])
)

cases = []
for value in ["max 100000", "100000 100000", "50000 100000", "25000 100000"]:
    cases.append(("cpu", value, value, "1073741824", "64"))
for label, value in [
    ("1G", "1073741824"),
    ("512M", "536870912"),
    ("256M", "268435456"),
    ("128M", "134217728"),
]:
    cases.append(("memory", label, "max 100000", value, "64"))
for value in ["64", "16", "4"]:
    cases.append(("pids", value, "max 100000", "1073741824", value))

results = []
with tempfile.TemporaryDirectory(prefix="mini-container-cgroup-") as temp_dir:
    model_directory = os.path.join(
        temp_dir, "model-store", "simple-classifier", "1.0.0"
    )
    shutil.copytree(
        os.path.join(project_root, "model-store", "simple-classifier", "1.0.0"),
        model_directory,
    )

    for index, (category, label, cpu_max, memory_max, pids_max) in enumerate(cases, 1):
        config = dict(base_config)
        config["model"] = dict(base_config["model"])
        config["model"]["host_path"] = model_directory
        config["cpu_max"] = cpu_max
        config["memory_max"] = memory_max
        config["pids_max"] = pids_max
        config["container_id"] = f"cgroup-ai-{os.getpid()}-{index}"

        config_path = os.path.join(temp_dir, f"config-{index}.json")
        with open(config_path, "w", encoding="utf-8") as output:
            json.dump(config, output, indent=2)
            output.write("\n")

        print(f"[{index}/{len(cases)}] {category}={label}", file=sys.stderr)
        environment = os.environ.copy()
        environment["MINI_CONTAINER_CONFIG"] = config_path
        completed = subprocess.run(
            [runtime],
            env=environment,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        try:
            runtime_result = json.loads(completed.stdout)
        except json.JSONDecodeError:
            if completed.stderr:
                print(completed.stderr, end="", file=sys.stderr)
            raise SystemExit(
                f"cgroup case produced no result JSON: {category}={label}, "
                f"exit={completed.returncode}"
            )
        if completed.returncode != 0:
            if runtime_result.get("status") != "constrained_failure":
                if completed.stderr:
                    print(completed.stderr, end="", file=sys.stderr)
                raise SystemExit(
                    f"unexpected cgroup case failure: {category}={label}, "
                    f"exit={completed.returncode}"
                )
            print(
                f"  constrained as expected: {runtime_result.get('error', 'child failed')}",
                file=sys.stderr,
            )
        elif completed.stderr:
            print(completed.stderr, end="", file=sys.stderr)

        inference = runtime_result.get("inference", {})
        result = runtime_result.get("result", {})
        results.append(
            {
                "case": {"category": category, "value": label},
                "status": runtime_result.get("status", "unknown"),
                "error": runtime_result.get("error"),
                "limits": {
                    "cpu_max": cpu_max,
                    "memory_max": memory_max,
                    "pids_max": pids_max,
                },
                "metrics": {
                    "runtime": runtime_result["runtime"],
                    "inference": inference,
                    "result": ({"class": result["class"]} if "class" in result else {}),
                    "cgroup": runtime_result["cgroup"],
                },
            }
        )

document = {
    "workload": "simple-classifier",
    "note": "No raw input data or model contents are included.",
    "runs": results,
}
with open(output_file, "w", encoding="utf-8") as output:
    json.dump(document, output, indent=2)
    output.write("\n")
json.dump(document, sys.stdout, indent=2)
sys.stdout.write("\n")
PY

echo "cgroup limit results saved: $OUTPUT_FILE" >&2
