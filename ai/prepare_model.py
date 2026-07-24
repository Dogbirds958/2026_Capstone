#!/usr/bin/env python3
"""Create and validate the simple classifier ONNX test workload."""

from __future__ import annotations

import json
import hashlib
from pathlib import Path

import numpy as np
import onnxruntime as ort
import torch
from torch import nn


MODEL_NAME = "simple-classifier"
MODEL_VERSION = "1.0.0"
INPUT_SIZE = 128
HIDDEN_SIZE = 16
OUTPUT_CLASSES = 3
INPUT_NAME = "input"
OUTPUT_NAME = "logits"

PROJECT_ROOT = Path(__file__).resolve().parent.parent
OUTPUT_DIR = PROJECT_ROOT / "model-store" / MODEL_NAME / MODEL_VERSION


class SimpleClassifier(nn.Module):
    """A deliberately small CPU workload for runtime integration tests."""

    def __init__(self) -> None:
        super().__init__()
        self.network = nn.Sequential(
            nn.Linear(INPUT_SIZE, HIDDEN_SIZE),
            nn.ReLU(),
            nn.Linear(HIDDEN_SIZE, OUTPUT_CLASSES),
        )

    def forward(self, input_tensor: torch.Tensor) -> torch.Tensor:
        return self.network(input_tensor)


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> None:
    torch.manual_seed(2026)
    np.random.seed(2026)

    model = SimpleClassifier().eval()
    example_input = torch.linspace(-1.0, 1.0, INPUT_SIZE, dtype=torch.float32).reshape(1, INPUT_SIZE)

    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    model_path = OUTPUT_DIR / "model.onnx"
    input_path = OUTPUT_DIR / "input.bin"

    with torch.no_grad():
        pytorch_logits = model(example_input).cpu().numpy()

    torch.onnx.export(
        model,
        example_input,
        model_path,
        input_names=[INPUT_NAME],
        output_names=[OUTPUT_NAME],
        opset_version=17,
        do_constant_folding=True,
        dynamo=False,
    )

    input_array = np.ascontiguousarray(example_input.cpu().numpy(), dtype=np.float32)
    input_array.tofile(input_path)

    labels = ["class-0", "class-1", "class-2"]
    write_json(OUTPUT_DIR / "labels.json", labels)

    manifest = {
        "name": MODEL_NAME,
        "version": MODEL_VERSION,
        "format": "onnx",
        "model_file": model_path.name,
        "input_file": input_path.name,
        "input_name": INPUT_NAME,
        "input_type": "float32",
        "input_shape": list(input_array.shape),
        "output_name": OUTPUT_NAME,
        "output_classes": OUTPUT_CLASSES,
        "sha256": {
            model_path.name: sha256_file(model_path),
            input_path.name: sha256_file(input_path),
        },
    }
    write_json(OUTPUT_DIR / "manifest.json", manifest)

    session = ort.InferenceSession(str(model_path), providers=["CPUExecutionProvider"])
    onnx_logits = session.run([OUTPUT_NAME], {INPUT_NAME: input_array})[0]
    np.testing.assert_allclose(onnx_logits, pytorch_logits, rtol=1e-5, atol=1e-6)

    predicted_index = int(np.argmax(onnx_logits, axis=1)[0])
    print(f"model: {model_path}")
    print(f"input: {input_path} ({input_path.stat().st_size} bytes)")
    print(f"ONNX Runtime providers: {session.get_providers()}")
    print(f"logits: {onnx_logits.tolist()}")
    print(f"predicted class: {predicted_index} ({labels[predicted_index]})")
    print("validation: PyTorch and ONNX Runtime outputs match")


if __name__ == "__main__":
    main()
