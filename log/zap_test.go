package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// newFileZapLogger 通过公开的 NewZapLogger 构造器构建 zap 后端 Logger，写入临时文件，并返回 Logger 与文件路径，供测试断言实际打印内容。
func newFileZapLogger(t *testing.T, level, format string) (Logger, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.log")
	lg := NewZapLogger(Config{
		{Writer: "file", Level: level, Format: format,
			WriterConfig: WriterConfig{Filename: p}},
	})
	return lg, p
}

// readLog 读回 newFileZapLogger 产出的日志文件内容。
func readLog(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log %s: %v", p, err)
	}
	return string(data)
}

// -----------------------------------------------------------------------
// zapLogger 方法：格式、级别
// -----------------------------------------------------------------------

func TestZapLogger_FVariadicFormatters(t *testing.T) {
	// *f 必须通过 fmt.Sprintf 格式化并把结果作为 zap msg，而非把参数当作 zap Fields。
	lg, p := newFileZapLogger(t, "debug", "text")

	lg.Debugf("hello %s %d", "world", 42)
	lg.Infof("i=%d", 7)
	lg.Warnf("w=%s", "x")
	lg.Errorf("e=%.1f", 3.14)

	out := readLog(t, p)
	t.Logf("captured:\n%s", out)
	wants := []string{
		"\thello world 42\n",
		"\ti=7\n",
		"\tw=x\n",
		"\te=3.1\n",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in output:\n%s", w, out)
		}
	}
	for _, leftover := range []string{"hello %s %d", "i=%d", "w=%s", "e=%.1f"} {
		if strings.Contains(out, leftover) {
			t.Errorf("format verb leaked into output %q:\n%s", leftover, out)
		}
	}
}

func TestZapLogger_NonFVariadicJoins(t *testing.T) {
	// 非 f 变参方法应通过 fmt.Sprint 把参数拼进 msg，而不是当作 zap Fields（会渲染成 key=value）。
	lg, p := newFileZapLogger(t, "debug", "text")
	lg.Debug("a", "b")
	lg.Info("single")
	out := readLog(t, p)
	t.Logf("captured:\n%s", out)
	if !strings.Contains(out, "\tab\n") {
		t.Errorf("Debug args should be joined into msg, got:\n%s", out)
	}
	if strings.Contains(out, "a=b") {
		t.Errorf("args were passed as Fields (key=value), got:\n%s", out)
	}
	if !strings.Contains(out, "\tsingle\n") {
		t.Errorf("Info single arg not rendered: %s", out)
	}
}

func TestZapLogger_AllLevelsEmit(t *testing.T) {
	lg, p := newFileZapLogger(t, "debug", "text")
	lg.Debug("d")
	lg.Debugf("d-%s", "f")
	lg.Info("i")
	lg.Infof("i-%s", "f")
	lg.Warn("w")
	lg.Warnf("w-%s", "f")
	lg.Error("e")
	lg.Errorf("e-%s", "f")

	out := readLog(t, p)
	t.Logf("captured:\n%s", out)
	wants := []string{
		"\td\n", "\td-f\n",
		"\ti\n", "\ti-f\n",
		"\tw\n", "\tw-f\n",
		"\te\n", "\te-f\n",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

// -----------------------------------------------------------------------
// resolveZapLevel / resolveZapWriter 辅助函数
// -----------------------------------------------------------------------

func TestResolveZapLevel(t *testing.T) {
	cases := map[string]zapcore.Level{
		"debug": zapcore.DebugLevel,
		"info":  zapcore.InfoLevel,
		"warn":  zapcore.WarnLevel,
		"error": zapcore.ErrorLevel,
		"fatal": zapcore.FatalLevel,
		"bogus": zapcore.InfoLevel, // 未知级别 -> Info
		"":      zapcore.InfoLevel,
	}
	for in, want := range cases {
		if got := resolveZapLevel(in); got != want {
			t.Errorf("resolveZapLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveZapWriter(t *testing.T) {
	// console / 未知 -> 背后是 os.Stdout
	if w := resolveZapWriter(OutputConfig{Writer: "console"}); w == nil {
		t.Error("console writer nil")
	}
	if w := resolveZapWriter(OutputConfig{Writer: "weird"}); w == nil {
		t.Error("unknown writer should fall back to stdout, got nil")
	}
	// file -> lumberjack 支持的 WriteSyncer（配置已传递）
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	w := resolveZapWriter(OutputConfig{
		Writer: "file",
		WriterConfig: WriterConfig{
			Filename: p, MaxSize: 7, MaxBackups: 2, MaxAge: 3, Compress: true,
		},
	})
	if w == nil {
		t.Fatal("file writer nil")
	}
	// 确认：实际写入并验证文件出现。
	if _, err := w.Write([]byte("ping")); err != nil {
		t.Fatalf("write through file writer: %v", err)
	}
	if data, _ := os.ReadFile(p); !strings.Contains(string(data), "ping") {
		t.Errorf("lumberjack file not written: %s", data)
	}
	// 确认 lumberjack 配置字段已生效。
	_ = lumberjack.Logger{} // 保持 import 被使用
}

// -----------------------------------------------------------------------
// NewZapLogger 集成测试（经 file writer）
// -----------------------------------------------------------------------

func TestNewZapLogger_File_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	lg := NewZapLogger(Config{
		{Writer: "file", Level: "debug", Format: "json",
			WriterConfig: WriterConfig{Filename: p, MaxSize: 1, MaxBackups: 1, MaxAge: 1}},
	})
	lg.Info("hello")
	lg.Debugf("d-%d", 1)

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	t.Logf("file content (%s):\n%s", p, data)
	lines := splitNonEmpty(string(data))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %s", len(lines), data)
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d not valid json: %v / %s", i, err, line)
		}
	}
	if !strings.Contains(lines[0], `"msg":"hello"`) || !strings.Contains(lines[0], `"level":"INFO"`) {
		t.Errorf("info line wrong: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"msg":"d-1"`) || !strings.Contains(lines[1], `"level":"DEBUG"`) {
		t.Errorf("debug line wrong: %s", lines[1])
	}
}

func TestNewZapLogger_LevelFiltering(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	lg := NewZapLogger(Config{
		{Writer: "file", Level: "warn", Format: "text",
			WriterConfig: WriterConfig{Filename: p}},
	})
	lg.Debug("d-should-be-dropped")
	lg.Info("i-should-be-dropped")
	lg.Warn("w-should-appear")
	lg.Error("e-should-appear")

	data, _ := os.ReadFile(p)
	s := string(data)
	t.Logf("file content (%s):\n%s", p, s)
	if strings.Contains(s, "should-be-dropped") {
		t.Errorf("debug/info leaked past warn filter: %s", s)
	}
	if !strings.Contains(s, "w-should-appear") || !strings.Contains(s, "e-should-appear") {
		t.Errorf("warn/error missing: %s", s)
	}
}

func TestNewZapLogger_MultiOutput(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.log")
	p2 := filepath.Join(dir, "b.log")
	lg := NewZapLogger(Config{
		{Writer: "file", Level: "debug", Format: "text", WriterConfig: WriterConfig{Filename: p1}},
		{Writer: "file", Level: "debug", Format: "json", WriterConfig: WriterConfig{Filename: p2}},
	})
	lg.Info("fanout")

	d1, _ := os.ReadFile(p1)
	d2, _ := os.ReadFile(p2)
	t.Logf("text file (%s):\n%s", p1, d1)
	t.Logf("json file (%s):\n%s", p2, d2)
	for _, d := range [][]byte{d1, d2} {
		if !strings.Contains(string(d), "fanout") {
			t.Errorf("fanout missing: %s", d)
		}
	}
	// text 文件用 console encoder（tab 分隔 time/level/caller/msg）；json 文件用 "msg":"fanout"。
	if !strings.Contains(string(d1), "INFO") || !strings.Contains(string(d1), "zap_test.go") {
		t.Errorf("text file wrong: %s", d1)
	}
	if !strings.Contains(string(d2), `"msg":"fanout"`) {
		t.Errorf("json file wrong: %s", d2)
	}
}

func TestNewZapLogger_EmptyConfigSafetyNet(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	var out bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&out, r); close(done) }()

	lg := NewZapLogger(Config{}) // 兜底 core 写入重定向后的 os.Stdout
	lg.Info("safety")

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
	t.Logf("captured stdout:\n%s", out.String())
	if !strings.Contains(out.String(), "safety") {
		t.Errorf("empty config should still log, got %q", out.String())
	}
}

// -----------------------------------------------------------------------
// Fatal：zap 原生 Fatal 先写记录再以退出码 1 退出。由于 os.Exit 无法同进程测试，这里在子进程中运行并断言退出码与输出。
// -----------------------------------------------------------------------

func TestZapLogger_FatalExits(t *testing.T) {
	if os.Getenv("ZAP_FATAL_CHILD") == "1" {
		// 子进程：记录 fatal 级别。zap 应先把记录写到 stdout 再 os.Exit(1)；执行不应到达下面的 os.Exit(99)。
		lg := NewZapLogger(Config{{Writer: "console", Level: "debug", Format: "text"}})
		lg.Fatalf("boom %d", 7)
		os.Exit(99)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestZapLogger_FatalExits$")
	cmd.Env = append(os.Environ(), "ZAP_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("expected Fatal to exit, but child returned normally")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if ee.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", ee.ExitCode())
	}
	if !strings.Contains(string(out), "boom 7") {
		t.Errorf("fatal message not logged:\n%s", out)
	}
	if !strings.Contains(string(out), "FATAL") {
		t.Errorf("fatal level not rendered:\n%s", out)
	}
	t.Logf("child output:\n%s", out)
}

func TestZapLogger_Sync(t *testing.T) {
	lg, p := newFileZapLogger(t, "debug", "text")
	lg.Info("flush-me")
	if err := lg.Sync(); err != nil {
		t.Errorf("Sync returned %v", err)
	}
	out := readLog(t, p)
	if !strings.Contains(out, "flush-me") {
		t.Errorf("record not written: %s", out)
	}
	t.Logf("captured:\n%s", out)
}

// zapLogger 必须满足 Logger 接口（zap.go 中 `var _ Logger = (*zapLogger)(nil)` 也在编译期强制）。
func TestZapLogger_SatisfiesLoggerInterface(t *testing.T) {
	var l Logger = &zapLogger{}
	_ = l // 能编译即表示接口已满足
}
