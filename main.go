package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type Config struct {
	Hostname    string   `json:"hostname"`     //container hostname
	Rootfs      string   `json:"rootfs"`       //root filesystem path
	SetupScript string   `json:"setup_script"` //rootfs setup script
	MemoryMax   string   `json:"memory_max"`   //memory max
	PidsMax     string   `json:"pids_max"`     //pid max
	Command     []string `json:"command"`      //command
	ContainerID string   `json:"container_id"` //dir name
	CpuMax      string   `json:"cpu_max"`      //cpu max
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
	fmt.Println("parrent process")

	//load config
	cfg := loadConfig()

	if err := runSetupScript(cfg); err != nil {
		fmt.Println("setup script fail:", err)
		os.Exit(1)
	}

	// 자식 프로세스 실행 객체 생성
	cmd := exec.Command("/proc/self/exe", "child")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	//	use namespace
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | //hostname
			syscall.CLONE_NEWPID | //pid namespace
			syscall.CLONE_NEWNS | //mount namespace
			syscall.CLONE_NEWNET, // network namespace
	}

	// 자식 프로세스 생성
	if err := cmd.Start(); err != nil {
		fmt.Println("start fail:", err)
		os.Exit(1)
	}
	fmt.Println("child pid:", cmd.Process.Pid)

	// cgroup cleanup
	defer os.RemoveAll(filepath.Join("/sys/fs/cgroup", cfg.ContainerID))

	// cgroup 설정
	if err := setCgroup(cmd.Process.Pid, cfg); err != nil {
		fmt.Println("cgroup fail:", err)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}

	// 부모가 자식 프로세스 기다림
	if err := cmd.Wait(); err != nil {
		fmt.Println("wait fail:", err)
		os.Exit(1)
	}
}

// child process
func child() {
	fmt.Println("child process")
	cfg := loadConfig()

	// hostname set
	if err := syscall.Sethostname([]byte(cfg.Hostname)); err != nil {
		fmt.Println("hostname fail:", err)
		os.Exit(1)
	}

	// rootfs path
	rootfs := cfg.Rootfs
	if !filepath.IsAbs(rootfs) {
		rootfs = filepath.Join(configDir(), rootfs)
	}

	if _, err := os.Stat(rootfs); err != nil {
		fmt.Println("rootfs not found:", rootfs)
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

	// exit
	// if exec command is empty
	if len(cfg.Command) == 0 {
		fmt.Println("need command set")
		os.Exit(1)
	}

	// container command exec
	if err := syscall.Exec(cfg.Command[0], cfg.Command, os.Environ()); err != nil {
		fmt.Println("shell exec fail:", err)
		os.Exit(1)
	}
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

func runSetupScript(cfg Config) error {
	if cfg.SetupScript == "" {
		return nil
	}

	scriptPath := cfg.SetupScript
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(configDir(), scriptPath)
	}

	rootfs := cfg.Rootfs
	if !filepath.IsAbs(rootfs) {
		rootfs = filepath.Join(configDir(), rootfs)
	}

	cmd := exec.Command("/bin/sh", scriptPath, rootfs)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func loadConfig() Config {

	//find config.json
	configPath := filepath.Join(configDir(), "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Println("config read fail:", err)
		os.Exit(1)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Println("config parse fail:", err)
		os.Exit(1)
	}

	//execption
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
	//prototype
	return cfg
}

func configDir() string {
	if _, err := os.Stat("config.json"); err == nil {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Println("working dir fail:", err)
			os.Exit(1)
		}

		return wd
	}

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("executable path fail:", err)
		os.Exit(1)
	}

	return filepath.Dir(exePath)
}
