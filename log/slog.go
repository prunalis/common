package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// slogLevel 将包内的字符串级别常量映射到 slog.Level。未出现在此映射中的级别会兜底为 slog.LevelInfo（见 resolveLevel），避免未知级别静默退化为 Debug。
var slogLevel = map[string]slog.Level{
	LevelDebug: slog.LevelDebug, // -4
	LevelInfo:  slog.LevelInfo,  //  0
	LevelWarn:  slog.LevelWarn,  //  4
	LevelError: slog.LevelError, //  8
	LevelFatal: slog.Level(12),  // 内置最高严重级别
}

// fatalLevel 是 Fatal/Fatalf 发出的级别。
const fatalLevel = slog.Level(12)

// exitFunc 在 Fatal 时终止进程；设为变量以便测试替换、避免真的退出。
var exitFunc = os.Exit

// NewSlogLogger 根据配置构建 Logger。cfg 中每项生成一个 slog.Handler，通过 slog.NewMultiHandler（Go 1.26 起可用）组合，一次日志调用会扇出到所有输出。
func NewSlogLogger(cfg Config) Logger {
	handlers := make([]slog.Handler, 0, len(cfg))
	writers := make([]io.Writer, 0, len(cfg))
	for _, c := range cfg {
		writer := resolveWriter(c)
		writers = append(writers, writer)
		opts := &slog.HandlerOptions{Level: resolveLevel(c.Level)}

		switch c.Format {
		case "json":
			handlers = append(handlers, slog.NewJSONHandler(writer, opts))
		default:
			// "text" 及未知格式兜底为 text。
			handlers = append(handlers, slog.NewTextHandler(writer, opts))
		}
	}

	// 兜底：绝不返回一个静默丢弃所有记录的 logger。
	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		writers = append(writers, os.Stdout)
	}

	return &slogLogger{
		logger:  slog.New(slog.NewMultiHandler(handlers...)),
		writers: writers,
	}
}

// resolveWriter 返回输出配置对应的 io.Writer；未知 writer 类型兜底为 stdout，避免 nil writer 导致 panic。
func resolveWriter(c OutputConfig) io.Writer {
	switch c.Writer {
	case "console":
		return os.Stdout
	case "file":
		return &lumberjack.Logger{
			Filename:   filepath.Join(c.WriterConfig.LogPath, c.WriterConfig.Filename),
			MaxSize:    c.WriterConfig.MaxSize,
			MaxBackups: c.WriterConfig.MaxBackups,
			MaxAge:     c.WriterConfig.MaxAge,
			Compress:   c.WriterConfig.Compress,
		}
	default:
		return os.Stdout
	}
}

// resolveLevel 将级别字符串映射为 slog.Level，未知值兜底为 Info。
func resolveLevel(level string) slog.Level {
	if lvl, ok := slogLevel[level]; ok {
		return lvl
	}
	return slog.LevelInfo
}

type slogLogger struct {
	logger  *slog.Logger
	writers []io.Writer
}

// syncer 由可刷新缓冲数据的 writer 实现（如 *os.File、*bufio.Writer）；lumberjack.Logger 未实现，会被跳过。
type syncer interface {
	Sync() error
}

// Sync 尽力刷新所有底层 writer。slog handler 是同步写入，内置 writer 基本无需刷新，仅当 writer 被缓冲包装时才有意义；错误被忽略（如管道上的 os.Stdout.Sync），因为 slog 无可丢失的缓冲。
func (s *slogLogger) Sync() error {
	for _, w := range s.writers {
		if sy, ok := w.(syncer); ok {
			_ = sy.Sync()
		}
	}
	return nil
}

// 变参（非 f）方法遵循主流 Go 日志库（logrus、zap.SugaredLogger）的 Print 风格：参数用空格拼接；f 变体用 fmt.Sprintf。这样两套 API 语义一致，也避免把参数当作 slog 的 key-value 处理。

func (s *slogLogger) Debug(args ...any) {
	s.logger.Debug(fmt.Sprint(args...))
}

func (s *slogLogger) Debugf(format string, args ...any) {
	s.logger.Debug(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Info(args ...any) {
	s.logger.Info(fmt.Sprint(args...))
}

func (s *slogLogger) Infof(format string, args ...any) {
	s.logger.Info(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Warn(args ...any) {
	s.logger.Warn(fmt.Sprint(args...))
}

func (s *slogLogger) Warnf(format string, args ...any) {
	s.logger.Warn(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Error(args ...any) {
	s.logger.Error(fmt.Sprint(args...))
}

func (s *slogLogger) Errorf(format string, args ...any) {
	s.logger.Error(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Fatal(args ...any) {
	s.logger.Log(context.Background(), fatalLevel, fmt.Sprint(args...))
	exitFunc(1)
}

func (s *slogLogger) Fatalf(format string, args ...any) {
	s.logger.Log(context.Background(), fatalLevel, fmt.Sprintf(format, args...))
	exitFunc(1)
}
