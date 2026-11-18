package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

// 本文件是 T2 的环境与状态隔离 helper。设计约束：
//   - 快照必须在测试赋值之前注册（isolateCmdGlobals 必须是测试第一步）；
//   - cleanup 恢复调用前原值，绝不写死默认值；
//   - pflag 切片用 SliceValue.GetSlice/Replace，普通 Value 用 String()/Set()；
//   - Cobra 无公开 Args getter，仅用无 unsafe 的窄反射读取 []string；
//   - Out/Err/In 保留原 nil 继承状态，不把当时的进程流永久固化。

// isolateAgents 把 detectAgents 固定为给定 Agent 集合。
// 未知名字直接失败：夹具错误必须显式暴露，不能静默退化为空集合。
func isolateAgents(t *testing.T, names ...string) {
	t.Helper()
	resolved := make([]tool.Tool, 0, len(names))
	for _, name := range names {
		tl := tool.ToolByName(name)
		if tl == nil {
			t.Fatalf("isolateAgents: unknown agent %q", name)
		}
		resolved = append(resolved, *tl)
	}
	prev := detectAgents
	detectAgents = func() []tool.Tool {
		out := make([]tool.Tool, len(resolved))
		copy(out, resolved)
		return out
	}
	t.Cleanup(func() { detectAgents = prev })
}

// isolateEnv 把 HOME 指到临时目录并刷新 home 缓存。
//
// 恢复必须由同一个 cleanup 完成：先还原 HOME，再刷新 home 缓存。若把
// home.ResetForTest 单独注册在 t.Setenv 之后，它会先于 HOME 还原执行，
// 结果缓存被重置回临时目录，等于没有恢复。
func isolateEnv(t *testing.T) string {
	t.Helper()
	return setTestHome(t, t.TempDir())
}

// setTestHome keeps the environment and the cached home directory in sync
// through cleanup, including fixtures that need to know the chosen path.
func setTestHome(t *testing.T, dir string) string {
	t.Helper()
	prevHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("isolateEnv: setting HOME: %v", err)
	}
	home.ResetForTest()
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", prevHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		home.ResetForTest()
	})
	return dir
}

// isolatePath 把 PATH 收窄为"仅含显式保留程序"的临时目录。
//
// 默认只保留 git：宿主 aivo 因此不可见（aivo.Detect 返回未安装，status 不再
// 执行真实 aivo 子进程），宿主 Agent 二进制也不可见。这不是清空 PATH，而是
// 显式保留清单；需要的程序必须逐个列出，缺失时立即失败。
//
// POSIX 用符号链接；Windows 复制二进制（符号链接需要额外权限）。
func isolatePath(t *testing.T, keepBins ...string) string {
	t.Helper()
	if len(keepBins) == 0 {
		keepBins = []string{"git"}
	}
	dir := t.TempDir()
	for _, name := range keepBins {
		// 在改写 PATH 之前解析真实路径，避免读到本 helper 自己生成的目录。
		resolved, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("isolatePath: required binary %q not found in PATH: %v", name, err)
		}
		dest := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			dest += ".exe"
			if err := copyExecutable(resolved, dest); err != nil {
				t.Fatalf("isolatePath: copying %q: %v", name, err)
			}
			continue
		}
		if err := os.Symlink(resolved, dest); err != nil {
			t.Fatalf("isolatePath: linking %q: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// isolateCmdGlobals 在测试赋值之前快照所有会被测试改写的包级状态，并在 cleanup
// 恢复。必须在测试第一步调用；在赋值之后调用会捕获已被污染的值。
func isolateCmdGlobals(t *testing.T) {
	t.Helper()

	prevRegistry, prevData, prevProfiles := RegistryDir, DataDir, ProfilesDir
	prevPlanDir, prevPlanJSON, prevPlanCheck, prevPlanApply := planDir, planJSON, planCheck, planApply
	prevPlanProfile, prevPlanSkills, prevPlanYes := planProfile, planSkills, planYes
	prevPlanOut := planOut
	prevInstallDir, prevInstallGlobal, prevInstallCopy := installDir, installGlobal, installCopy
	prevInstallAgents, prevInstallSkills := installAgents, installSkills
	prevInstallProfile, prevInstallFromReg := installProfile, installFromReg
	prevListAgents, prevListProject, prevListDir := listAgents, listProject, listDir
	prevListGlobal, prevListJSON := listGlobal, listJSON
	prevRmForce, prevRmProject, prevRmDir := rmForce, rmProject, rmDir
	prevRmAll, prevRmYes, prevRmIsMCP := rmAll, rmYes, rmIsMCP
	prevRmAgents, prevRmSkills := rmAgents, rmSkills
	prevUpdateGlobal, prevUpdateProject, prevUpdateDir := updateGlobal, updateProject, updateDir
	prevUpdateYes, prevUpdateRegistry, prevUpdateInPlace := updateYes, updateRegistry, updateInPlace

	cmds := []*cobra.Command{rootCmd, planCmd, installCmd, listCmd, rmCmd, statusCmd, updateCmd}
	streams := make([]rootStreams, 0, len(cmds))
	for _, cmd := range cmds {
		streams = append(streams, snapshotRootStreams(cmd))
	}
	flagSnap := snapshotFlags(t, cmds...)

	t.Cleanup(func() {
		// 先恢复 flag 值：rootCmd 的持久化 flag 直接绑定 RegistryDir 等变量，
		// 若在变量之后恢复会把已还原的变量重新写回快照时的临时值。
		flagSnap.restore()
		for _, stream := range streams {
			stream.restore()
		}
		RegistryDir, DataDir, ProfilesDir = prevRegistry, prevData, prevProfiles
		planDir, planJSON, planCheck, planApply = prevPlanDir, prevPlanJSON, prevPlanCheck, prevPlanApply
		planProfile, planSkills, planYes = prevPlanProfile, prevPlanSkills, prevPlanYes
		planOut = prevPlanOut
		installDir, installGlobal, installCopy = prevInstallDir, prevInstallGlobal, prevInstallCopy
		installAgents, installSkills = prevInstallAgents, prevInstallSkills
		installProfile, installFromReg = prevInstallProfile, prevInstallFromReg
		listAgents, listProject, listDir = prevListAgents, prevListProject, prevListDir
		listGlobal, listJSON = prevListGlobal, prevListJSON
		rmForce, rmProject, rmDir = prevRmForce, prevRmProject, prevRmDir
		rmAll, rmYes, rmIsMCP = prevRmAll, prevRmYes, prevRmIsMCP
		rmAgents, rmSkills = prevRmAgents, prevRmSkills
		updateGlobal, updateProject, updateDir = prevUpdateGlobal, prevUpdateProject, prevUpdateDir
		updateYes, updateRegistry, updateInPlace = prevUpdateYes, prevUpdateRegistry, prevUpdateInPlace
	})
}

// rootStreams 记录 Cobra 命令的 Args 与 Out/Err/In 的原始状态。
// 三者都可能是"未设置"（nil），此时 Cobra 继承进程默认流；恢复时必须还原 nil，
// 否则会把当前进程流永久固化到命令上。
type rootStreams struct {
	cmd      *cobra.Command
	args     []string
	outNil   bool
	errNil   bool
	inNil    bool
	outValue io.Writer
	errValue io.Writer
	inValue  io.Reader
}

// snapshotRootStreams 快照命令的 args 与输出流状态。
func snapshotRootStreams(cmd *cobra.Command) rootStreams {
	s := rootStreams{cmd: cmd, args: cobraArgs(cmd)}
	s.outNil = cobraFieldIsNil(cmd, "outWriter")
	s.errNil = cobraFieldIsNil(cmd, "errWriter")
	s.inNil = cobraFieldIsNil(cmd, "inReader")
	if !s.outNil {
		s.outValue = cmd.OutOrStdout()
	}
	if !s.errNil {
		s.errValue = cmd.ErrOrStderr()
	}
	if !s.inNil {
		s.inValue = cmd.InOrStdin()
	}
	return s
}

func (s rootStreams) restore() {
	s.cmd.SetArgs(s.args)
	if s.outNil {
		s.cmd.SetOut(nil)
	} else {
		s.cmd.SetOut(s.outValue)
	}
	if s.errNil {
		s.cmd.SetErr(nil)
	} else {
		s.cmd.SetErr(s.errValue)
	}
	if s.inNil {
		s.cmd.SetIn(nil)
	} else {
		s.cmd.SetIn(s.inValue)
	}
}

// cobraArgs 用窄反射读取 cobra.Command 未导出的 args 字段（无 unsafe，只读）。
// Cobra 没有公开的 Args getter，而恢复必须还原调用前的参数切片。
func cobraArgs(cmd *cobra.Command) []string {
	v := reflect.ValueOf(cmd)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	f := v.Elem().FieldByName("args")
	if !f.IsValid() || f.Kind() != reflect.Slice || f.Type().Elem().Kind() != reflect.String || f.IsNil() {
		return nil
	}
	// 未导出字段不能用 Interface()（会 panic）；逐元素 String() 是安全的只读访问。
	out := make([]string, f.Len())
	for i := 0; i < f.Len(); i++ {
		out[i] = f.Index(i).String()
	}
	return out
}

// cobraFieldIsNil 判断命令的输出流字段是否为 nil（nil 表示继承进程默认流）。
func cobraFieldIsNil(cmd *cobra.Command, name string) bool {
	v := reflect.ValueOf(cmd)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return true
	}
	f := v.Elem().FieldByName(name)
	if !f.IsValid() {
		return true
	}
	switch f.Kind() {
	case reflect.Interface, reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
		return f.IsNil()
	default:
		return false
	}
}

// flagSnapshot 记录若干命令上全部 flag 的值与 Changed 状态。
// seen 记录快照覆盖到的 flag；Cobra 会在首次 Execute 时惰性创建 --version
// 等 flag，这些后建 flag 的 Changed 也需要复位，否则会留给后续用例。
type flagSnapshot struct {
	items []flagSnapshotItem
	seen  map[*pflag.Flag]bool
	cmds  []*cobra.Command
}

type flagSnapshotItem struct {
	flag    *pflag.Flag
	changed bool
	slice   []string // 非 nil 表示 SliceValue 快照
	str     string   // 普通 Value 快照
}

// snapshotFlags 快照给定命令的 flag Value 与 Changed。
//
// 切片 flag 不能用 Value.String()+Set() 往返（String() 产出 "[a,b]" 形式），
// 必须用 SliceValue.GetSlice/Replace；其余用 String()/Set()。
func snapshotFlags(t *testing.T, cmds ...*cobra.Command) flagSnapshot {
	t.Helper()
	snap := flagSnapshot{seen: map[*pflag.Flag]bool{}, cmds: cmds}
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		collect := func(f *pflag.Flag) {
			snap.seen[f] = true
			item := flagSnapshotItem{flag: f, changed: f.Changed}
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				item.slice = append([]string{}, sv.GetSlice()...)
			} else {
				item.str = f.Value.String()
			}
			snap.items = append(snap.items, item)
		}
		cmd.Flags().VisitAll(collect)
		cmd.PersistentFlags().VisitAll(collect)
	}
	return snap
}

func (s flagSnapshot) restore() {
	// 先复位快照内 flag，再处理测试期间才创建的 flag（如 --version）：
	// 它们不在快照里，但值和 Changed 都会泄漏给下一个用例（--version 为 true
	// 会让后续 Execute 直接打印版本而不执行命令），必须复位到默认值。
	defer func() {
		for _, cmd := range s.cmds {
			if cmd == nil {
				continue
			}
			reset := func(f *pflag.Flag) {
				if !s.seen[f] {
					resetFlagToDefault(f)
				}
			}
			cmd.Flags().VisitAll(reset)
			cmd.PersistentFlags().VisitAll(reset)
		}
	}()
	for _, item := range s.items {
		if item.slice != nil {
			if sv, ok := item.flag.Value.(pflag.SliceValue); ok {
				if err := sv.Replace(item.slice); err != nil {
					continue
				}
			}
		} else if err := item.flag.Value.Set(item.str); err != nil {
			continue
		}
		item.flag.Changed = item.changed
	}
}

// copyExecutable 复制可执行文件，供 Windows 上无法建符号链接时使用。
func copyExecutable(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o755)
}

// tempDataDir 把 DataDir 指向临时目录并在 cleanup 恢复原值。
func tempDataDir(t *testing.T) string {
	t.Helper()
	prev := DataDir
	dir := t.TempDir()
	DataDir = dir
	t.Cleanup(func() { DataDir = prev })
	return dir
}

// resetFlagToDefault 把测试期间才创建的 flag 复位到默认值。
// 切片 flag 不能直接用 Value.Set(DefValue)：DefValue 是 "[a,b]" 形式，
// 必须走 SliceValue.Replace。
func resetFlagToDefault(f *pflag.Flag) {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		if f.DefValue == "" || f.DefValue == "[]" {
			_ = sv.Replace(nil)
		} else {
			trimmed := strings.TrimSuffix(strings.TrimPrefix(f.DefValue, "["), "]")
			_ = sv.Replace(strings.Split(trimmed, ","))
		}
	} else {
		_ = f.Value.Set(f.DefValue)
	}
	f.Changed = false
}
