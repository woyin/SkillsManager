package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 本文件提供跨平台"伪进程"：把当前测试二进制复制成目标程序名，被调用时由
// TestMain 早分派，按同名配置文件记录参数、返回指定退出码/延时/输出。
//
// 设计要点：
//   - 不使用 #!/bin/sh，Windows 可运行；
//   - 配置放在可执行文件旁边的 <name>.fake.json，而不是环境变量：同一测试
//     可能同时需要伪 git 与伪 aivo，环境变量会互相覆盖；
//   - 只有设置了 SM_FAKE_PROC=1 且同名配置文件存在时才接管，避免误吞真实
//     测试运行（go test 的二进制名通常不是 "git"/"aivo"）。

const (
	envFakeProc = "SM_FAKE_PROC"
	fakeSuffix  = ".fake.json"
)

// fakeProcessConfig 是伪进程的落盘配置。
type fakeProcessConfig struct {
	LogPath string `json:"log"`
	Exit    int    `json:"exit"`
	SleepMS int    `json:"sleep_ms"`
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
}

// fakeProcessOptions 描述伪进程行为。
type fakeProcessOptions struct {
	ExitCode int
	SleepMS  int
	Stdout   string
	Stderr   string
}

// TestMain 在被当作伪进程调用时提前分派，否则运行正常测试。
func TestMain(m *testing.M) {
	if code, handled := fakeProcessMain(); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// fakeProcessMain 判断本次进程是否是伪进程调用，并执行对应行为。
func fakeProcessMain() (int, bool) {
	if os.Getenv(envFakeProc) != "1" {
		return 0, false
	}
	// 必须用可执行文件的真实路径，而不是 os.Args[0]：exec.Command("git", ...)
	// 把 argv[0] 设为 "git"，相对路径无法定位同目录下的配置文件。
	exe, err := os.Executable()
	if err != nil {
		return 0, false
	}
	data, err := os.ReadFile(exe + fakeSuffix)
	if err != nil {
		return 0, false
	}
	var cfg fakeProcessConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0, false
	}
	if cfg.LogPath != "" {
		line := strings.Join(os.Args[1:], " ") + "\n"
		f, err := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(line)
			_ = f.Close()
		}
	}
	if cfg.Stdout != "" {
		fmt.Fprint(os.Stdout, cfg.Stdout)
	}
	if cfg.Stderr != "" {
		fmt.Fprint(os.Stderr, cfg.Stderr)
	}
	if cfg.SleepMS > 0 {
		time.Sleep(time.Duration(cfg.SleepMS) * time.Millisecond)
	}
	return cfg.Exit, true
}

// installFakeProcess 把当前测试二进制复制为 dir/name[.exe]，写入同名配置，
// 并返回伪程序目录、调用日志路径与读取日志的函数。
func installFakeProcess(t testing.TB, name string, opt fakeProcessOptions) (dir string, logPath string, read func() string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("installFakeProcess: locating test binary: %v", err)
	}
	dir = t.TempDir()
	dest := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		dest += ".exe"
	}
	if err := copyExecutable(self, dest); err != nil {
		t.Fatalf("installFakeProcess: copying %q to %q: %v", self, dest, err)
	}
	logPath = filepath.Join(t.TempDir(), name+".log")
	cfg := fakeProcessConfig{LogPath: logPath, Exit: opt.ExitCode, SleepMS: opt.SleepMS, Stdout: opt.Stdout, Stderr: opt.Stderr}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("installFakeProcess: encoding config: %v", err)
	}
	if err := os.WriteFile(dest+fakeSuffix, data, 0o644); err != nil {
		t.Fatalf("installFakeProcess: writing config: %v", err)
	}
	t.Setenv(envFakeProc, "1")

	return dir, logPath, func() string {
		logData, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("installFakeProcess: reading %q call log: %v", name, err)
		}
		return string(logData)
	}
}

// prependPath 把 dir 放到 PATH 之前，保留其余条目（真实 git 等仍可用）。
func prependPath(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
