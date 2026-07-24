# Simple classifier ONNX workload

The model preparation dependencies are installed into an isolated host-side
Python virtual environment. Python and PyTorch are used only to create and
validate the model; they are not copied into the container rootfs.

From the project root, run:

```sh
./setup_onnx_model.sh
```

The script creates `.venv-onnx-export`, installs pinned CPU-only dependencies,
generates the model artifacts, and verifies inference with ONNX Runtime.

To regenerate the model later with the prepared environment:

```sh
. .venv-onnx-export/bin/activate
python3 ai/prepare_model.py
```

## ONNX Runtime C++ SDK

Install the host C++ compiler and CMake when they are not already available:

```sh
./setup_build_tools.sh
. .venv-build-tools/bin/activate
```

Install the pinned prebuilt CPU SDK into `third_party/onnxruntime`:

```sh
./setup_onnxruntime_cpp.sh
```

Configure the C++ build by passing its location explicitly:

```sh
cmake \
  -DONNXRUNTIME_ROOT="$PWD/third_party/onnxruntime" \
  -S ai \
  -B build/ai

cmake --build build/ai --parallel
```

Run the host-side inference check:

```sh
build/ai/inference-runner \
  --model model-store/simple-classifier/1.0.0/model.onnx \
  --input model-store/simple-classifier/1.0.0/input.bin
```

The standalone runner prints JSON containing model-load, inference, total
runner timing, class, and logits. When launched through `mini-container`, the
Go parent adds namespace, cgroup, rootfs setup, and total container timings to
the final JSON document.

Repeat only `session.Run()` while reusing the loaded model, input, and tensor
to compare cold model loading with warm inference:

```sh
build/ai/inference-runner \
  --model model-store/simple-classifier/1.0.0/model.onnx \
  --input model-store/simple-classifier/1.0.0/input.bin \
  --repeat 100
```

The JSON includes every inference duration together with `repeat`,
`inference_avg_ms`, `inference_min_ms`, and `inference_max_ms`.

Inspect and copy the runner's resolved shared-library dependencies into the
BusyBox rootfs:

```sh
./scripts/copy_runtime_dependencies.sh
```

The optional arguments are the runner and rootfs paths, respectively:

```sh
./scripts/copy_runtime_dependencies.sh ./build/ai/inference-runner ./busybox-rootfs
```

If the rootfs was created by a privileged or namespace-mapped process and is
not writable by the current user, run the copy script with sufficient
permission, for example `sudo ./scripts/copy_runtime_dependencies.sh`.

Prepare the complete BusyBox inference rootfs, including a copied snapshot of
the simple classifier model:

```sh
sudo ./setup_busybox_rootfs.sh
```

Test inference directly in the chroot before integrating it with the Go
runtime:

```sh
sudo chroot busybox-rootfs \
  /bin/inference-runner \
  --model /models/current/model.onnx \
  --input /models/current/input.bin
```

After enabling the read-only model bind mount, run the automated isolation
test through the Go runtime:

```sh
sudo ./tests/test_model_readonly.sh
```

Run the AI workload against the CPU, memory, and PID cgroup limit matrix and
save the collected cgroup v2 statistics as JSON:

```sh
sudo ./tests/test_cgroup_limits.sh
```

The default output file is `build/cgroup-limit-results.json`. The report omits
raw input data and model contents.

## Test suite

Refresh the rootfs runner first, then run the tests individually:

```sh
./tests/test_host_inference.sh
sudo ./tests/test_chroot_inference.sh
sudo ./tests/test_runtime_inference.sh
sudo ./tests/test_model_readonly.sh
sudo ./tests/test_memory_limit.sh
sudo ./tests/test_cpu_limit.sh
sudo ./tests/test_missing_model.sh
```
