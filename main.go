package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ModelMountConfig struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
}

type Config struct {
	Hostname    string           `json:"hostname"`     //container hostname
	Rootfs      string           `json:"rootfs"`       //root filesystem path
	MemoryMax   string           `json:"memory_max"`   //memory max
	PidsMax     string           `json:"pids_max"`     //pid max
	Command     []string         `json:"command"`      //command
	ContainerID string           `json:"container_id"` //dir name
	CpuMax      string           `json:"cpu_max"`      //cpu max
	Model       ModelMountConfig `json:"model"`        //model bind mount
	configDir   string
}

type ModelManifest struct {
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Format    string            `json:"format"`
	ModelFile string            `json:"model_file"`
	InputFile string            `json:"input_file"`
	SHA256    map[string]string `json:"sha256"`
}

type ChildMetrics struct {
	RootfsSetupMS float64 `json:"rootfs_setup_ms"`
}

type InferenceMetrics struct {
	ModelLoadMS        float64   `json:"model_load_ms"`
	Repeat             int       `json:"repeat"`
	InferenceMS        float64   `json:"inference_ms"`
	InferenceAverageMS float64   `json:"inference_avg_ms"`
	InferenceMinimumMS float64   `json:"inference_min_ms"`
	InferenceMaximumMS float64   `json:"inference_max_ms"`
	InferenceRunsMS    []float64 `json:"inference_runs_ms"`
	RunnerTotalMS      float64   `json:"runner_total_ms"`
}

type InferenceResult struct {
	Class  int       `json:"class"`
	Logits []float64 `json:"logits"`
}

type RunnerOutput struct {
	Inference InferenceMetrics `json:"inference"`
	Result    InferenceResult  `json:"result"`
}

type RuntimeMetrics struct {
	RootfsSetupMS    float64 `json:"rootfs_setup_ms"`
	NamespaceStartMS float64 `json:"namespace_start_ms"`
	CgroupSetupMS    float64 `json:"cgroup_setup_ms"`
	ContainerTotalMS float64 `json:"container_total_ms"`
}

type CgroupMetrics struct {
	CPUMax        string            `json:"cpu_max"`
	MemoryMax     string            `json:"memory_max"`
	PidsMax       string            `json:"pids_max"`
	CPUStat       map[string]uint64 `json:"cpu_stat"`
	MemoryCurrent uint64            `json:"memory_current"`
	MemoryPeak    uint64            `json:"memory_peak"`
	MemoryEvents  map[string]uint64 `json:"memory_events"`
	PidsCurrent   uint64            `json:"pids_current"`
}

type ContainerOutput struct {
	Status    string           `json:"status"`
	Error     string           `json:"error,omitempty"`
	Runtime   RuntimeMetrics   `json:"runtime"`
	Inference InferenceMetrics `json:"inference"`
	Result    InferenceResult  `json:"result"`
	Cgroup    CgroupMetrics    `json:"cgroup"`
}

func main() {

	// if exec child process
	// call child() & container reset
	if len(os.Args) > 1 && os.Args[1] == "child" {
		child()
		return
	}

	run() //parent process
}

// parnet process
func run() {
	containerStart := time.Now()
	fmt.Fprintln(os.Stderr, "parent process")

	//load config
	cfg := loadConfig()

	// AI모델 무결성 검사
	if err := validateModelManifest(cfg); err != nil {
		fmt.Println("model manifest validation fail:", err)
		os.Exit(1)
	}

	// 현재 프로세스가 root 권한인지 검사
	if os.Geteuid() != 0 {
		fmt.Println("root privilege required: run with sudo because this runtime uses namespaces, chroot, mount, and cgroup")
		os.Exit(1)
	}

	// 자식 프로세스 실행 객체 생성
	cmd := exec.Command("/proc/self/exe", "child")

	// 자식 프로세스의 파일 디스크립터를 부모와 연결
	cmd.Stdin = os.Stdin
	var runnerOutput bytes.Buffer
	cmd.Stdout = &runnerOutput
	cmd.Stderr = os.Stderr

	// 자식 -> 부모 데이터 전달 파이프
	// 자식이 컨테이너 rootfs 준비에 걸린 시간을 부모에게 전송 (CHILDMetrics 구조체)
	// 굳이 필요한가...? 결과측정용?
	metricsReader, metricsWriter, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "metrics pipe fail:", err)
		os.Exit(1)
	}

	// 부모 -> 자식 데이터 전달 파이프
	// 아직 자식의 cgroup이 등록되지 않았기 떄문에 자식을 대기시키기 위한 용도
	// 세마포어 느낌?
	startGateReader, startGateWriter, err := os.Pipe()
	if err != nil {
		metricsReader.Close()
		metricsWriter.Close()
		fmt.Fprintln(os.Stderr, "start gate pipe fail:", err)
		os.Exit(1)
	}

	// 자식이게 파일 디스크립터 전달
	cmd.ExtraFiles = []*os.File{metricsWriter, startGateReader}

	// namespace 설정
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | //hostname
			syscall.CLONE_NEWPID | //pid namespace
			syscall.CLONE_NEWNS | //mount namespace
			syscall.CLONE_NEWNET, // network namespace
	}

	// 자식 프로세스 생성 && 시작
	namespaceStart := time.Now()
	if err := cmd.Start(); err != nil {
		metricsReader.Close()
		metricsWriter.Close()
		startGateReader.Close()
		startGateWriter.Close()
		fmt.Println("start fail:", err)
		os.Exit(1)
	}
	namespaceStartMS := elapsedMilliseconds(namespaceStart) // namespace 생성 시간 계산
	metricsWriter.Close()
	startGateReader.Close()
	fmt.Fprintln(os.Stderr, "child pid:", cmd.Process.Pid)

	// cgroup cleanup
	// cgroup 디텍터리 삭제 예약
	defer os.RemoveAll(filepath.Join("/sys/fs/cgroup", cfg.ContainerID))

	// cgroup 설정
	cgroupStart := time.Now()
	if err := setCgroup(cmd.Process.Pid, cfg); err != nil {
		metricsReader.Close()
		startGateWriter.Close()
		fmt.Println("cgroup fail:", err)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}
	cgroupSetupMS := elapsedMilliseconds(cgroupStart) // cgroup 생성 시간 계산

	// 자식 프로세스 시작 (cgroup 설정 끝)
	if _, err := startGateWriter.Write([]byte{1}); err != nil {
		metricsReader.Close()
		startGateWriter.Close()
		_ = cmd.Process.Kill()
		fmt.Fprintln(os.Stderr, "release child start gate fail:", err)
		os.Exit(1)
	}
	startGateWriter.Close()

	// 자식 프로세스 종료 대기 (추론 완료)
	waitErr := cmd.Wait()

	// 추론에 사용된 cgroup 통계 수집
	cgroupMetrics, err := collectCgroupMetrics(filepath.Join("/sys/fs/cgroup", cfg.ContainerID), cfg)
	if err != nil {
		metricsReader.Close()
		fmt.Fprintln(os.Stderr, "collect cgroup metrics fail:", err)
		os.Exit(1)
	}

	// 자식 프로세스에서 실행된 rootfs 준비 시간 측정값 수집
	childMetricsData, err := io.ReadAll(metricsReader)
	metricsReader.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read child metrics fail:", err)
		os.Exit(1)
	}
	var childMetrics ChildMetrics
	if len(childMetricsData) > 0 {
		if err := json.Unmarshal(childMetricsData, &childMetrics); err != nil {
			fmt.Fprintln(os.Stderr, "parse child metrics fail:", err)
			os.Exit(1)
		}
	} else if waitErr == nil {
		fmt.Fprintln(os.Stderr, "parse child metrics fail: metrics are empty")
		os.Exit(1)
	}

	// 자식이 내보낸 결과값을 파싱하지 않고 그대로 stdout에 출력
	// 나중에 다른 프로그램과 연결하기 위한 용도
	// 지금은 안쓸듯?
	if os.Getenv("MINI_CONTAINER_RAW_OUTPUT") == "1" {
		if _, err := os.Stdout.Write(runnerOutput.Bytes()); err != nil {
			fmt.Fprintln(os.Stderr, "write raw container output fail:", err)
			os.Exit(1)
		}
		if waitErr != nil {
			os.Exit(processExitCode(waitErr))
		}
		return
	}

	// 최종 출력 구조체 생성
	output := ContainerOutput{
		Status: "success",
		Runtime: RuntimeMetrics{
			RootfsSetupMS:    childMetrics.RootfsSetupMS,
			NamespaceStartMS: namespaceStartMS,
			CgroupSetupMS:    cgroupSetupMS,
			ContainerTotalMS: elapsedMilliseconds(containerStart),
		},
		Cgroup: cgroupMetrics,
	}

	// 자식이 비정상 종료했을 경우 실패처리
	if waitErr != nil {
		output.Status = "constrained_failure"
		output.Error = waitErr.Error()
		if err := writeContainerOutput(output); err != nil {
			fmt.Fprintln(os.Stderr, "encode result fail:", err)
			os.Exit(1)
		}
		os.Exit(processExitCode(waitErr))
	}

	// 추론 결과 json 파싱
	var inferenceOutput RunnerOutput
	if err := json.Unmarshal(runnerOutput.Bytes(), &inferenceOutput); err != nil {
		fmt.Fprintln(os.Stderr, "parse inference output fail:", err)
		os.Exit(1)
	}

	// 최종 출력 구조체에 합치기
	output.Inference = inferenceOutput.Inference
	output.Result = inferenceOutput.Result

	// 전체 실행 시간 계산
	output.Runtime.ContainerTotalMS = elapsedMilliseconds(containerStart)
	if err := writeContainerOutput(output); err != nil {
		fmt.Fprintln(os.Stderr, "encode result fail:", err)
		os.Exit(1)
	}
}

func writeContainerOutput(output ContainerOutput) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func processExitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if exitCode := exitError.ExitCode(); exitCode >= 0 {
			return exitCode
		}
	}
	return 1
}

// 모델 검증
func validateModelManifest(cfg Config) error {

	modelStorePath, err := filepath.EvalSymlinks(filepath.Join(cfg.configDir, "model-store"))
	if err != nil {
		return fmt.Errorf("resolve model-store: %w", err)
	}
	modelStorePath, err = filepath.Abs(modelStorePath)
	if err != nil {
		return fmt.Errorf("make model-store absolute: %w", err)
	}
	if err := requirePathInside(modelStorePath, cfg.Model.HostPath, "model host_path"); err != nil {
		return err
	}

	manifestPath := filepath.Join(cfg.Model.HostPath, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest.json: %w", err)
	}

	var manifest ModelManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest.json: %w", err)
	}
	if manifest.Format != "onnx" {
		return fmt.Errorf("unsupported model format %q: expected onnx", manifest.Format)
	}
	if manifest.ModelFile == "" {
		return fmt.Errorf("manifest model_file is empty")
	}
	if manifest.InputFile == "" {
		return fmt.Errorf("manifest input_file is empty")
	}

	modelPath, err := resolveManifestFile(cfg.Model.HostPath, manifest.ModelFile)
	if err != nil {
		return fmt.Errorf("validate model_file: %w", err)
	}
	inputPath, err := resolveManifestFile(cfg.Model.HostPath, manifest.InputFile)
	if err != nil {
		return fmt.Errorf("validate input_file: %w", err)
	}

	filesToVerify := map[string]string{
		manifest.ModelFile: modelPath,
		manifest.InputFile: inputPath,
	}
	for manifestName, path := range filesToVerify {
		expectedHash, ok := manifest.SHA256[manifestName]
		if !ok || expectedHash == "" {
			return fmt.Errorf("sha256 missing for %s", manifestName)
		}
		actualHash, err := fileSHA256(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", manifestName, err)
		}
		if !strings.EqualFold(expectedHash, actualHash) {
			return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", manifestName, expectedHash, actualHash)
		}
	}

	return nil
}

func resolveManifestFile(modelDirectory, manifestName string) (string, error) {
	if filepath.IsAbs(manifestName) {
		return "", fmt.Errorf("path must be relative: %s", manifestName)
	}
	for _, component := range strings.Split(manifestName, string(filepath.Separator)) {
		if component == ".." {
			return "", fmt.Errorf("path must not contain '..': %s", manifestName)
		}
	}

	path, err := filepath.EvalSymlinks(filepath.Join(modelDirectory, manifestName))
	if err != nil {
		return "", fmt.Errorf("file does not exist: %s: %w", manifestName, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file %s: %w", manifestName, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file: %s", manifestName)
	}
	if err := requirePathInside(modelDirectory, path, "manifest file"); err != nil {
		return "", err
	}
	return path, nil
}

func requirePathInside(parent, candidate, description string) error {
	relativePath, err := filepath.Rel(parent, candidate)
	if err != nil {
		return fmt.Errorf("compare %s with allowed directory: %w", description, err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return fmt.Errorf("%s is outside %s: %s", description, parent, candidate)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// child process
func child() {
	if err := waitForCgroup(); err != nil {
		fmt.Fprintln(os.Stderr, "wait for cgroup fail:", err)
		os.Exit(1)
	}
	rootfsSetupStart := time.Now()
	fmt.Fprintln(os.Stderr, "child process")
	cfg := loadConfig()

	// hostname set
	if err := syscall.Sethostname([]byte(cfg.Hostname)); err != nil {
		fmt.Println("hostname fail:", err)
		os.Exit(1)
	}

	// rootfs path
	rootfs := cfg.Rootfs
	rootfsInfo, err := os.Stat(rootfs)
	if err != nil {
		fmt.Println("rootfs not found:", rootfs)
		os.Exit(1)
	}
	if !rootfsInfo.IsDir() {
		fmt.Println("rootfs is not a directory:", rootfs)
		os.Exit(1)
	}

	// Prevent mount events from propagating back to the host mount namespace.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		fmt.Println("mount propagation setup fail:", err)
		os.Exit(1)
	}

	if err := mountModel(rootfs, cfg.Model); err != nil {
		fmt.Println("model mount fail:", err)
		os.Exit(1)
	}

	// rootfs change
	if err := syscall.Chroot(rootfs); err != nil {
		fmt.Println("chroot fail:", err)
		os.Exit(1)
	}

	// move working dir
	if err := os.Chdir("/"); err != nil {
		fmt.Println("chdir fail:", err)
		os.Exit(1)
	}

	// proc mount
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		fmt.Println("/proc mount fail:", err)
		os.Exit(1)
	}

	if err := writeChildMetrics(ChildMetrics{RootfsSetupMS: elapsedMilliseconds(rootfsSetupStart)}); err != nil {
		fmt.Fprintln(os.Stderr, "write child metrics fail:", err)
		os.Exit(1)
	}

	if len(cfg.Command) == 0 {
		cfg.Command = []string{"/bin/sh"}
	}

	if err := syscall.Exec(cfg.Command[0], cfg.Command, os.Environ()); err != nil {
		fmt.Println("command exec fail:", err)
		os.Exit(1)
	}
}

func waitForCgroup() error {
	file := os.NewFile(4, "container-start-gate")
	if file == nil {
		return fmt.Errorf("start gate file descriptor is unavailable")
	}
	defer file.Close()
	buffer := []byte{0}
	if _, err := io.ReadFull(file, buffer); err != nil {
		return err
	}
	if buffer[0] != 1 {
		return fmt.Errorf("invalid start gate signal")
	}
	return nil
}

func writeChildMetrics(metrics ChildMetrics) error {
	file := os.NewFile(3, "container-metrics")
	if file == nil {
		return fmt.Errorf("metrics file descriptor is unavailable")
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(metrics)
}

func elapsedMilliseconds(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds()) / float64(time.Millisecond)
}

func collectCgroupMetrics(cgroupPath string, cfg Config) (CgroupMetrics, error) {
	cpuStat, err := readCgroupKeyValues(filepath.Join(cgroupPath, "cpu.stat"))
	if err != nil {
		return CgroupMetrics{}, err
	}
	memoryCurrent, err := readCgroupUint(filepath.Join(cgroupPath, "memory.current"))
	if err != nil {
		return CgroupMetrics{}, err
	}
	memoryPeak, err := readCgroupUint(filepath.Join(cgroupPath, "memory.peak"))
	if err != nil {
		return CgroupMetrics{}, err
	}
	memoryEvents, err := readCgroupKeyValues(filepath.Join(cgroupPath, "memory.events"))
	if err != nil {
		return CgroupMetrics{}, err
	}
	pidsCurrent, err := readCgroupUint(filepath.Join(cgroupPath, "pids.current"))
	if err != nil {
		return CgroupMetrics{}, err
	}
	return CgroupMetrics{
		CPUMax:        cfg.CpuMax,
		MemoryMax:     cfg.MemoryMax,
		PidsMax:       cfg.PidsMax,
		CPUStat:       cpuStat,
		MemoryCurrent: memoryCurrent,
		MemoryPeak:    memoryPeak,
		MemoryEvents:  memoryEvents,
		PidsCurrent:   pidsCurrent,
	}, nil
}

func readCgroupUint(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return value, nil
}

func readCgroupKeyValues(path string) (map[string]uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	values := make(map[string]uint64)
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("parse %s line %d: expected key and value", path, lineNumber+1)
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %s key %s: %w", path, fields[0], err)
		}
		values[fields[0]] = value
	}
	return values, nil
}

func mountModel(rootfs string, model ModelMountConfig) error {
	rootfsRealPath, err := filepath.EvalSymlinks(rootfs)
	if err != nil {
		return fmt.Errorf("resolve rootfs: %w", err)
	}
	rootfsRealPath, err = filepath.Abs(rootfsRealPath)
	if err != nil {
		return fmt.Errorf("make rootfs absolute: %w", err)
	}

	targetPath, err := secureMountTarget(rootfsRealPath, model.ContainerPath)
	if err != nil {
		return err
	}

	if err := syscall.Mount(model.HostPath, targetPath, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind %s to %s: %w", model.HostPath, targetPath, err)
	}

	if model.ReadOnly {
		flags := uintptr(syscall.MS_BIND | syscall.MS_REMOUNT | syscall.MS_RDONLY)
		if err := syscall.Mount("", targetPath, "", flags, ""); err != nil {
			return fmt.Errorf("remount model read-only: %w", err)
		}
	}

	return nil
}

func secureMountTarget(rootfsRealPath, containerPath string) (string, error) {
	if !filepath.IsAbs(containerPath) {
		return "", fmt.Errorf("model container_path must be absolute: %s", containerPath)
	}
	if strings.IndexByte(containerPath, 0) >= 0 {
		return "", fmt.Errorf("model container_path contains a NUL byte")
	}
	for _, component := range strings.Split(containerPath, string(filepath.Separator)) {
		if component == ".." {
			return "", fmt.Errorf("model container_path must not contain '..': %s", containerPath)
		}
	}

	cleanContainerPath := filepath.Clean(containerPath)
	if cleanContainerPath == string(filepath.Separator) {
		return "", fmt.Errorf("model container_path must not be the root directory")
	}

	relativeTarget := strings.TrimPrefix(cleanContainerPath, string(filepath.Separator))
	currentPath := rootfsRealPath
	for _, component := range strings.Split(relativeTarget, string(filepath.Separator)) {
		currentPath = filepath.Join(currentPath, component)
		info, err := os.Lstat(currentPath)
		if os.IsNotExist(err) {
			if err := os.Mkdir(currentPath, 0755); err != nil {
				return "", fmt.Errorf("create model mount directory %s: %w", currentPath, err)
			}
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect model mount path %s: %w", currentPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("model mount path contains a symbolic link: %s", currentPath)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("model mount path component is not a directory: %s", currentPath)
		}
	}

	targetRealPath, err := filepath.EvalSymlinks(currentPath)
	if err != nil {
		return "", fmt.Errorf("resolve model mount target: %w", err)
	}
	relativeToRootfs, err := filepath.Rel(rootfsRealPath, targetRealPath)
	if err != nil {
		return "", fmt.Errorf("compare model mount target with rootfs: %w", err)
	}
	if relativeToRootfs == ".." || strings.HasPrefix(relativeToRootfs, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeToRootfs) {
		return "", fmt.Errorf("model mount target escapes rootfs: %s", targetRealPath)
	}

	return targetRealPath, nil
}

func setCgroup(pid int, cfg Config) error {
	//dir path
	cgroupPath := filepath.Join("/sys/fs/cgroup", cfg.ContainerID)

	// cgroup dir create
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return err
	}

	// set memory max
	if err := os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte(cfg.MemoryMax), 0644); err != nil {
		return err
	}

	// set pid max
	if err := os.WriteFile(filepath.Join(cgroupPath, "pids.max"), []byte(cfg.PidsMax), 0644); err != nil {
		return err
	}

	// set cpu max
	if err := os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte(cfg.CpuMax), 0644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(fmt.Sprint(pid)), 0644); err != nil {
		return err
	}

	return nil
}

// config 파일 로드
func loadConfig() Config {

	// config.json 위치를 환경변수로 받기
	// 환경변수에 없으면 해당 디렉터리에서 찾기
	// 솔직히 필요 없을듯?
	configPath := os.Getenv("MINI_CONTAINER_CONFIG")
	explicitConfig := configPath != ""
	if !explicitConfig {
		configPath = "config.json"
	}

	// 수정필요
	// 굳이 환경변수로 넣지 말고 그냥 해당 디렉터리에서 찾게끔 수정
	data, err := os.ReadFile(configPath)
	if err != nil && !explicitConfig {
		exePath, exeErr := os.Executable()
		if exeErr != nil {
			fmt.Println("executable path fail:", exeErr)
			os.Exit(1)
		}

		configPath = filepath.Join(filepath.Dir(exePath), "config.json")
		data, err = os.ReadFile(configPath)
		if err != nil {
			fmt.Println("config read fail:", err)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Println("config read fail:", err)
		os.Exit(1)
	}

	// 구조체 정의
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Println("config parse fail:", err)
		os.Exit(1)
	}

	// config.json 예외처리
	if cfg.ContainerID == "" {
		fmt.Println("config error: container_id is empty")
		os.Exit(1)
	}
	if cfg.Hostname == "" {
		fmt.Println("config error: hostname is empty")
		os.Exit(1)
	}
	if cfg.Rootfs == "" {
		fmt.Println("config error: rootfs is empty")
		os.Exit(1)
	}
	if cfg.MemoryMax == "" {
		fmt.Println("config error: memory_max is empty")
		os.Exit(1)
	}
	if cfg.PidsMax == "" {
		fmt.Println("config error: pids_max is empty")
		os.Exit(1)
	}
	if cfg.CpuMax == "" {
		fmt.Println("config error: cpu_max is empty")
		os.Exit(1)
	}
	if cfg.Model.HostPath == "" {
		fmt.Println("config error: model.host_path is empty")
		os.Exit(1)
	}
	if cfg.Model.ContainerPath == "" {
		fmt.Println("config error: model.container_path is empty")
		os.Exit(1)
	}

	// config.json 파일에 rootfs 위치값이 상대경로 일 경우 절대경로로 변경
	// ex) ./rootfs  => /home/user/capstone/rootfs
	if !filepath.IsAbs(cfg.Rootfs) {
		cfg.Rootfs = filepath.Clean(filepath.Join(filepath.Dir(configPath), cfg.Rootfs))
	}
	configDirectory, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		fmt.Println("config error: resolve config directory:", err)
		os.Exit(1)
	}
	cfg.configDir = configDirectory
	if !filepath.IsAbs(cfg.Model.HostPath) {
		cfg.Model.HostPath = filepath.Clean(filepath.Join(filepath.Dir(configPath), cfg.Model.HostPath))
	}

	// AI모델 호스트 경로를 절대경로로 변경 & 모델 경로 검사?
	modelHostPath, err := filepath.EvalSymlinks(cfg.Model.HostPath)
	if err != nil {
		fmt.Println("config error: resolve model.host_path:", err)
		os.Exit(1)
	}
	modelInfo, err := os.Stat(modelHostPath)
	if err != nil {
		fmt.Println("config error: stat model.host_path:", err)
		os.Exit(1)
	}
	if !modelInfo.IsDir() {
		fmt.Println("config error: model.host_path is not a directory:", cfg.Model.HostPath)
		os.Exit(1)
	}
	cfg.Model.HostPath, err = filepath.Abs(modelHostPath)
	if err != nil {
		fmt.Println("config error: make model.host_path absolute:", err)
		os.Exit(1)
	}
	if !filepath.IsAbs(cfg.Model.ContainerPath) {
		fmt.Println("config error: model.container_path must be absolute")
		os.Exit(1)
	}
	for _, component := range strings.Split(cfg.Model.ContainerPath, string(filepath.Separator)) {
		if component == ".." {
			fmt.Println("config error: model.container_path must not contain '..'")
			os.Exit(1)
		}
	}
	if len(cfg.Command) == 0 {
		cfg.Command = []string{"/bin/sh"}
	}

	return cfg
}
