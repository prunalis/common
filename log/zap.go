package log

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// zapLevel 将包内的字符串级别常量映射到 zapcore.Level。未知级别兜底为 InfoLevel（见 resolveZapLevel）。
var zapLevel = map[string]zapcore.Level{
	LevelDebug: zapcore.DebugLevel, // -1
	LevelInfo:  zapcore.InfoLevel,  //  0
	LevelWarn:  zapcore.WarnLevel,  //  1
	LevelError: zapcore.ErrorLevel, //  2
	LevelFatal: zapcore.FatalLevel, //  5
}

// NewZapLogger 构建基于 go.uber.org/zap 的 Logger。cfg 中每项生成一个 zapcore.Core，通过 zapcore.NewTee 组合，一次日志调用扇出到所有输出，与 NewSlogLogger 对称。
//
// Fatal 使用 zap 原生终结行为：记录 FatalLevel 后以退出码 1 终止进程（os.Exit）。
func NewZapLogger(cfg Config) Logger {
	cores := make([]zapcore.Core, 0, len(cfg))
	for _, c := range cfg {
		cores = append(cores, zapcore.NewCore(
			resolveZapEncoder(c.Format),
			resolveZapWriter(c),
			resolveZapLevel(c.Level),
		))
	}

	// 兜底：绝不返回一个静默丢弃所有记录的 logger。
	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(
			resolveZapEncoder("text"),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		))
	}

	return &zapLogger{
		// AddCaller 记录调用位置，AddCallerSkip(1) 跳过自身的转发方法，使 caller 指向用户代码。
		logger: zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddCallerSkip(1)),
	}
}

// resolveZapWriter 返回输出配置对应的 zapcore.WriteSyncer；未知 writer 类型兜底为 stdout。
func resolveZapWriter(c OutputConfig) zapcore.WriteSyncer {
	switch c.Writer {
	case "console":
		return zapcore.AddSync(os.Stdout)
	case "file":
		return zapcore.AddSync(&lumberjack.Logger{
			Filename:   c.WriterConfig.Filename,
			MaxSize:    c.WriterConfig.MaxSize,
			MaxBackups: c.WriterConfig.MaxBackups,
			MaxAge:     c.WriterConfig.MaxAge,
			Compress:   c.WriterConfig.Compress,
		})
	default:
		return zapcore.AddSync(os.Stdout)
	}
}

// resolveZapEncoder 按格式选择 encoder："json" 输出 JSON，"text" 及未知格式兜底为 console（text）encoder。
func resolveZapEncoder(format string) zapcore.Encoder {
	enc := zapEncoderConfig()
	switch format {
	case "json":
		return zapcore.NewJSONEncoder(enc)
	default:
		return zapcore.NewConsoleEncoder(enc)
	}
}

// resolveZapLevel 将级别字符串映射为 zapcore.Level，未知值兜底为 Info。
func resolveZapLevel(level string) zapcore.Level {
	if lvl, ok := zapLevel[level]; ok {
		return lvl
	}
	return zapcore.InfoLevel
}

// zapEncoderConfig 是 JSON 与 console encoder 共用的编码配置："2006-01-02 15:04:05" 时间、大写级别名、msg 作为消息 key、caller 记录调用位置。
func zapEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        zapcore.OmitKey,
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  zapcore.OmitKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

type zapLogger struct {
	logger *zap.Logger
}

// 编译期接口一致性检查。
var _ Logger = (*zapLogger)(nil)

// 与 slogLogger 一致：变参方法用 fmt.Sprint 拼接、f 变体用 fmt.Sprintf，结果作为 zap 消息。

func (z *zapLogger) Debug(args ...any) {
	z.logger.Debug(fmt.Sprint(args...))
}

func (z *zapLogger) Debugf(format string, args ...any) {
	z.logger.Debug(fmt.Sprintf(format, args...))
}

func (z *zapLogger) Info(args ...any) {
	z.logger.Info(fmt.Sprint(args...))
}

func (z *zapLogger) Infof(format string, args ...any) {
	z.logger.Info(fmt.Sprintf(format, args...))
}

func (z *zapLogger) Warn(args ...any) {
	z.logger.Warn(fmt.Sprint(args...))
}

func (z *zapLogger) Warnf(format string, args ...any) {
	z.logger.Warn(fmt.Sprintf(format, args...))
}

func (z *zapLogger) Error(args ...any) {
	z.logger.Error(fmt.Sprint(args...))
}

func (z *zapLogger) Errorf(format string, args ...any) {
	z.logger.Error(fmt.Sprintf(format, args...))
}

// Fatal 记录 FatalLevel 后以退出码 1 终止进程（zap 原生行为）。注意这使 zapLogger.Fatal 无法同进程测试（os.Exit），不同于使用可注入 exitFunc 的 slogLogger。
func (z *zapLogger) Fatal(args ...any) {
	z.logger.Fatal(fmt.Sprint(args...))
}

// Fatalf 格式化后经 zap 原生 Fatal 记录 FatalLevel。*zap.Logger 没有 Fatalf（仅 SugaredLogger 有），故手动 Sprintf。
func (z *zapLogger) Fatalf(format string, args ...any) {
	z.logger.Fatal(fmt.Sprintf(format, args...))
}

// Sync 通过 zap 自身机制刷新缓冲记录。注意在 stdout/stderr 不可 seek 的平台（如管道）上 Sync 会返回错误，调用方通常忽略：`_ = log.Sync()`。
func (z *zapLogger) Sync() error {
	return z.logger.Sync()
}
