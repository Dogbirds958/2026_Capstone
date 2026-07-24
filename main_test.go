package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestManifest(t *testing.T, modelDirectory string, modelHash, inputHash string) {
	t.Helper()
	manifest := ModelManifest{
		Name:      "simple-classifier",
		Version:   "1.0.0",
		Format:    "onnx",
		ModelFile: "model.onnx",
		InputFile: "input.bin",
		SHA256: map[string]string{
			"model.onnx": modelHash,
			"input.bin":  inputHash,
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDirectory, "manifest.json"), data, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestValidateModelManifest(t *testing.T) {
	projectRoot := t.TempDir()
	modelDirectory := filepath.Join(projectRoot, "model-store", "simple-classifier", "1.0.0")
	if err := os.MkdirAll(modelDirectory, 0755); err != nil {
		t.Fatalf("create model directory: %v", err)
	}
	modelPath := filepath.Join(modelDirectory, "model.onnx")
	inputPath := filepath.Join(modelDirectory, "input.bin")
	if err := os.WriteFile(modelPath, []byte("model"), 0644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if err := os.WriteFile(inputPath, []byte("input"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	modelHash, err := fileSHA256(modelPath)
	if err != nil {
		t.Fatalf("hash model: %v", err)
	}
	inputHash, err := fileSHA256(inputPath)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	writeTestManifest(t, modelDirectory, modelHash, inputHash)

	cfg := Config{configDir: projectRoot, Model: ModelMountConfig{HostPath: modelDirectory}}
	if err := validateModelManifest(cfg); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	writeTestManifest(t, modelDirectory, strings.Repeat("0", 64), inputHash)
	if err := validateModelManifest(cfg); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("error = %v, want sha256 mismatch", err)
	}
}

func TestValidateModelManifestRejectsModelOutsideStore(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectRoot, "model-store"), 0755); err != nil {
		t.Fatalf("create model-store: %v", err)
	}
	outside := filepath.Join(projectRoot, "outside")
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatalf("create outside directory: %v", err)
	}

	cfg := Config{configDir: projectRoot, Model: ModelMountConfig{HostPath: outside}}
	if err := validateModelManifest(cfg); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("error = %v, want model-store boundary rejection", err)
	}
}

func TestSecureMountTargetCreatesSafeDirectory(t *testing.T) {
	rootfs := t.TempDir()
	target, err := secureMountTarget(rootfs, "/models/current")
	if err != nil {
		t.Fatalf("secureMountTarget returned an error: %v", err)
	}
	want := filepath.Join(rootfs, "models", "current")
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("target directory was not created: info=%v err=%v", info, err)
	}
}

func TestSecureMountTargetRejectsUnsafePaths(t *testing.T) {
	rootfs := t.TempDir()
	tests := []struct {
		name          string
		containerPath string
		message       string
	}{
		{name: "relative", containerPath: "models/current", message: "must be absolute"},
		{name: "parent traversal", containerPath: "/models/../outside", message: "must not contain '..'"},
		{name: "root", containerPath: "/", message: "must not be the root"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := secureMountTarget(rootfs, test.containerPath)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestSecureMountTargetRejectsSymbolicLink(t *testing.T) {
	rootfs := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(rootfs, "models")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := secureMountTarget(rootfs, "/models/current")
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("error = %v, want symbolic link rejection", err)
	}
}
