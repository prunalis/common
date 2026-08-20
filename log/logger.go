package log

import (
	"fmt"
	"sync"
)

var (
	mu      sync.RWMutex
	loggers = make(map[string]Logger)

	defaultLogger Logger
)

// DefaultLoggerName 是默认 logger 的名称，在 init 时注册。
const DefaultLoggerName = "default"

func init() {
	Register(DefaultLoggerName, NewZapLogger(defaultConfig))
}

// Register 注册一个命名 logger。重复注册非默认名称会 panic；重新注册 DefaultLoggerName 则替换默认 logger。
func Register(name string, logger Logger) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := loggers[name]; ok && name != DefaultLoggerName {
		panic(fmt.Sprintf("logger name[%s] already register", name))
	}
	loggers[name] = logger
	if name == DefaultLoggerName {
		defaultLogger = logger
	}
}

// Get 返回指定名称的 logger；未注册时兜底返回默认 logger（因此永不返回 nil）。
func Get(name string) Logger {
	mu.RLock()
	defer mu.RUnlock()
	if l, ok := loggers[name]; ok {
		return l
	}
	// 兜底返回默认 logger，而不是返回 nil 接口，避免首次调用即 panic。
	return defaultLogger
}

// GetDefaultLogger 返回默认 logger。
func GetDefaultLogger() Logger {
	mu.RLock()
	logger := defaultLogger
	mu.RUnlock()
	return logger
}

// Logger 是 slogLogger 和 zapLogger 共同实现的日志接口。变参方法用 fmt.Sprint 拼接参数，*f 变体用 fmt.Sprintf 格式化；Fatal/Fatalf 记录 fatal 级别后终止进程；Sync 刷新缓冲的记录。
type Logger interface {
	Debug(args ...any)
	Debugf(format string, args ...any)
	Info(args ...any)
	Infof(format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Sync() error
}

// Debug 使用默认 logger 记录 debug 级别日志。
func Debug(args ...any) {
	GetDefaultLogger().Debug(args...)
}

// Debugf 使用默认 logger 记录格式化的 debug 级别日志。
func Debugf(format string, args ...any) {
	GetDefaultLogger().Debugf(format, args...)
}

// Info 使用默认 logger 记录 info 级别日志。
func Info(args ...any) {
	GetDefaultLogger().Info(args...)
}

// Infof 使用默认 logger 记录格式化的 info 级别日志。
func Infof(format string, args ...any) {
	GetDefaultLogger().Infof(format, args...)
}

// Warn 使用默认 logger 记录 warn 级别日志。
func Warn(args ...any) {
	GetDefaultLogger().Warn(args...)
}

// Warnf 使用默认 logger 记录格式化的 warn 级别日志。
func Warnf(format string, args ...any) {
	GetDefaultLogger().Warnf(format, args...)
}

// Error 使用默认 logger 记录 error 级别日志。
func Error(args ...any) {
	GetDefaultLogger().Error(args...)
}

// Errorf 使用默认 logger 记录格式化的 error 级别日志。
func Errorf(format string, args ...any) {
	GetDefaultLogger().Errorf(format, args...)
}

// Fatal 记录 fatal 级别日志并终止进程。
func Fatal(args ...any) {
	GetDefaultLogger().Fatal(args...)
}

// Fatalf 记录格式化的 fatal 级别日志并终止进程。
func Fatalf(format string, args ...any) {
	GetDefaultLogger().Fatalf(format, args...)
}

// Sync 刷新所有已注册的 logger（不只默认），返回遇到的第一个错误（如有）。
func Sync() error {
	mu.RLock()
	defer mu.RUnlock()

	var firstErr error
	for _, l := range loggers {
		if err := l.Sync(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
