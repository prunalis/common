package log

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

// syncWriter 是一个记录 Sync 是否被调用的 io.Writer。
type syncWriter struct {
	bytes.Buffer
	synced bool
}

func (s *syncWriter) Sync() error { s.synced = true; return nil }

// newFileSlogLogger 通过公开的 NewSlogLogger 构造器构建 slog 后端 Logger，写入临时文件，并返回 Logger 与文件路径，供测试断言实际打印内容。
func newFileSlogLogger(t *testing.T, level, format string) (Logger, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.log")
	lg := NewSlogLogger(Config{
		{Writer: "file", Level: level, Format: format,
			WriterConfig: WriterConfig{Filename: p}},
	})
	return lg, p
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// -----------------------------------------------------------------------
// slogLogger 方法：格式、级别
// -----------------------------------------------------------------------

// *f 格式化 bug 的回归测试。此前 Debugf 把格式串当 slog msg、把参数当 key-value，输出类似 msg="hello %s %d" world=42。修复方式是 fmt.Sprintf(format, ...)。
func TestSlogLogger_FVariadicFormatters(t *testing.T) {
	lg, p := newFileSlogLogger(t, "debug", "text")

	lg.Debugf("hello %s %d", "world", 42)
	lg.Infof("i=%d", 7)
	lg.Warnf("w=%s", "x")
	lg.Errorf("e=%.1f", 3.14)

	out := readLog(t, p)
	t.Logf("captured:\n%s", out)
	wants := []string{
		`level=DEBUG msg="hello world 42"`,
		`level=INFO msg="i=7"`,
		`level=WARN msg="w=x"`,
		`level=ERROR msg="e=3.1"`,
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

func TestSlogLogger_NonFVariadicJoins(t *testing.T) {
	// 非 f 变参方法应通过 fmt.Sprint 把参数拼进 msg，而不是当作 slog 的 key-value（旧 bug）。
	lg, p := newFileSlogLogger(t, "debug", "text")
	lg.Debug("a", "b")
	lg.Info("single")
	out := readLog(t, p)
	if !strings.Contains(out, "msg=ab") {
		t.Errorf("Debug args should be joined into msg, got:\n%s", out)
	}
	if strings.Contains(out, "a=b") {
		t.Errorf("args were passed as key-value pairs (old bug), got:\n%s", out)
	}
	if !strings.Contains(out, "msg=single") {
		t.Errorf("Info single arg not rendered: %s", out)
	}
	t.Logf("captured:\n%s", out)
}

func TestSlogLogger_AllLevelsEmit(t *testing.T) {
	lg, p := newFileSlogLogger(t, "debug", "text")
	lg.Debug("d")
	lg.Debugf("d-%s", "f")
	lg.Info("i")
	lg.Infof("i-%s", "f")
	lg.Warn("w")
	lg.Warnf("w-%s", "f")
	lg.Error("e")
	lg.Errorf("e-%s", "f")

	out := readLog(t, p)
	wants := []string{
		`level=DEBUG msg=d`, `msg=d-f`,
		`level=INFO msg=i`, `msg=i-f`,
		`level=WARN msg=w`, `msg=w-f`,
		`level=ERROR msg=e`, `msg=e-f`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in output:\n%s", w, out)
		}
	}
	t.Logf("captured:\n%s", out)
}

// -----------------------------------------------------------------------
// resolveLevel / resolveWriter 辅助函数
// -----------------------------------------------------------------------

func TestResolveLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"fatal": slog.Level(12),
		"bogus": slog.LevelInfo, // 未知级别 -> Info（而非静默退化为 Debug）
		"":      slog.LevelInfo,
	}
	for in, want := range cases {
		if got := resolveLevel(in); got != want {
			t.Errorf("resolveLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveWriter(t *testing.T) {
	if w := resolveWriter(OutputConfig{Writer: "console"}); w != os.Stdout {
		t.Errorf("console should be os.Stdout, got %T", w)
	}
	if w := resolveWriter(OutputConfig{Writer: "weird-unknown"}); w != os.Stdout {
		t.Errorf("unknown writer should fall back to os.Stdout, got %T", w)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	w := resolveWriter(OutputConfig{
		Writer: "file",
		WriterConfig: WriterConfig{
			Filename: p, MaxSize: 7, MaxBackups: 2, MaxAge: 3, Compress: true,
		},
	})
	lj, ok := w.(*lumberjack.Logger)
	if !ok {
		t.Fatalf("file writer should be *lumberjack.Logger, got %T", w)
	}
	if lj.Filename != p || lj.MaxSize != 7 || lj.MaxBackups != 2 || lj.MaxAge != 3 || !lj.Compress {
		t.Errorf("lumberjack config not propagated: %+v", lj)
	}
}

// -----------------------------------------------------------------------
// NewSlogLogger 集成测试（经 file writer）
// -----------------------------------------------------------------------

func TestNewSlogLogger_File_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	lg := NewSlogLogger(Config{
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

func TestNewSlogLogger_LevelFiltering(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	lg := NewSlogLogger(Config{
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

func TestNewSlogLogger_MultiOutput(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.log")
	p2 := filepath.Join(dir, "b.log")
	lg := NewSlogLogger(Config{
		{Writer: "file", Level: "debug", Format: "text", WriterConfig: WriterConfig{Filename: p1}},
		{Writer: "file", Level: "debug", Format: "json", WriterConfig: WriterConfig{Filename: p2}},
	})
	lg.Info("fanout")

	for _, p := range []string{p1, p2} {
		d, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(d), "fanout") {
			t.Errorf("%s missing fanout: %s", p, d)
		}
	}
	// text 文件应是 key=value 形式，json 文件应是 "msg":"fanout" 形式。
	d1, _ := os.ReadFile(p1)
	d2, _ := os.ReadFile(p2)
	t.Logf("text file (%s):\n%s", p1, d1)
	t.Logf("json file (%s):\n%s", p2, d2)
	if !strings.Contains(string(d1), `msg=fanout`) {
		t.Errorf("text file wrong: %s", d1)
	}
	if !strings.Contains(string(d2), `"msg":"fanout"`) {
		t.Errorf("json file wrong: %s", d2)
	}
}

// 空配置也应通过兜底逻辑产生可用的 logger（验证不会静默丢弃所有记录）。
func TestNewSlogLogger_EmptyConfigSafetyNet(t *testing.T) {
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

	lg := NewSlogLogger(Config{}) // 兜底 handler 捕获重定向后的 os.Stdout
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
// Fatal：exitFunc 必须以 1 被调用，且消息必须被记录。
// -----------------------------------------------------------------------

func TestSlogLogger_FatalLogsAndExits(t *testing.T) {
	lg, p := newFileSlogLogger(t, "debug", "text")

	orig := exitFunc
	var gotCode int
	exitFunc = func(c int) { gotCode = c }
	t.Cleanup(func() { exitFunc = orig })

	lg.Fatalf("boom %d", 7)
	if gotCode != 1 {
		t.Errorf("exit code = %d, want 1", gotCode)
	}
	out := readLog(t, p)
	if !strings.Contains(out, `msg="boom 7"`) {
		t.Errorf("Fatalf message not logged: %s", out)
	}
	// slog 将未知数字级别（12）渲染为 ERROR+offset。
	if !strings.Contains(out, "level=ERROR+4") {
		t.Errorf("Fatalf level not rendered as ERROR+4: %s", out)
	}
	t.Logf("fatalf captured:\n%s", out)

	gotCode = 0
	lg.Fatal("x", 1)
	if gotCode != 1 {
		t.Errorf("Fatal exit code = %d, want 1", gotCode)
	}
	out = readLog(t, p)
	if !strings.Contains(out, `msg=x1`) {
		t.Errorf("Fatal message not logged: %s", out)
	}
	t.Logf("fatal captured:\n%s", out)
}

// -----------------------------------------------------------------------
// Register / Get / 包级函数
// -----------------------------------------------------------------------

func TestGet_FallsBackToDefault(t *testing.T) {
	if got := Get("definitely-not-registered"); got == nil {
		t.Error("Get(unknown) returned nil, want default logger")
	}
	if got := GetDefaultLogger(); got == nil {
		t.Error("GetDefaultLogger returned nil")
	}
}

func TestRegister_AndGet(t *testing.T) {
	name := "test-logger-x"
	var buf bytes.Buffer
	l := &slogLogger{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	Register(name, l)
	t.Cleanup(func() {
		mu.Lock()
		delete(loggers, name)
		mu.Unlock()
	})
	if got := Get(name); got != l {
		t.Errorf("Get(%s) returned wrong logger", name)
	}
}

func TestRegister_DuplicateNamePanics(t *testing.T) {
	name := "dup-test"
	Register(name, &slogLogger{logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer func() {
		mu.Lock()
		delete(loggers, name)
		mu.Unlock()
	}()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register, got none")
		}
	}()
	Register(name, &slogLogger{logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func TestRegister_DefaultOverwriteIsAllowed(t *testing.T) {
	orig := GetDefaultLogger()
	t.Cleanup(func() { Register(DefaultLoggerName, orig) })
	var buf bytes.Buffer
	l := &slogLogger{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	Register(DefaultLoggerName, l)
	if GetDefaultLogger() != l {
		t.Error("default logger was not overwritten")
	}
}

func TestSlogLogger_Sync(t *testing.T) {
	sw := &syncWriter{}
	s := &slogLogger{
		logger:  slog.New(slog.NewTextHandler(sw, &slog.HandlerOptions{Level: slog.LevelDebug})),
		writers: []io.Writer{sw},
	}
	s.Info("flush-me")
	if err := s.Sync(); err != nil {
		t.Errorf("Sync returned %v", err)
	}
	if !sw.synced {
		t.Error("Sync did not call the writer's Sync")
	}
	if !strings.Contains(sw.String(), "flush-me") {
		t.Errorf("record not written: %s", sw.String())
	}
	t.Logf("captured:\n%s", sw.String())
}

func TestPackageSync_AllRegisteredLoggers(t *testing.T) {
	// 包级 Sync 必须刷新所有已注册 logger，而非仅默认。
	sw := &syncWriter{}
	l := &slogLogger{
		logger:  slog.New(slog.NewTextHandler(sw, &slog.HandlerOptions{Level: slog.LevelDebug})),
		writers: []io.Writer{sw},
	}
	name := "sync-target"
	Register(name, l)
	t.Cleanup(func() {
		mu.Lock()
		delete(loggers, name)
		mu.Unlock()
	})

	// Sync 返回遇到的第一个错误。默认 logger 可能是 zap 后端、对 os.Stdout 的 Sync 会失败（"bad file descriptor"），故忽略错误、只断言遍历到了我们的命名 logger。
	_ = Sync()
	if !sw.synced {
		t.Error("package Sync did not call Sync on the named logger")
	}
}

func TestPackageFunctions_UseDefault(t *testing.T) {
	orig := GetDefaultLogger()
	t.Cleanup(func() { Register(DefaultLoggerName, orig) })
	var buf bytes.Buffer
	Register(DefaultLoggerName,
		&slogLogger{logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))})

	Info("pkg-", 1)
	Debugf("d-%s", "f")
	Warn("w")
	Errorf("e-%d", 2)

	out := buf.String()
	for _, want := range []string{
		`level=INFO msg=pkg-1`,
		`level=DEBUG msg=d-f`,
		`level=WARN msg=w`,
		`level=ERROR msg=e-2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	t.Logf("default buffer:\n%s", out)
}
