package errs

import (
	"errors"
	"fmt"
)

type RetCode int32

type Error struct {
	Code RetCode
	Msg  string
}

// ErrCode permits any integer defined in
type ErrCode interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~int | ~uintptr
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("code:%d msg:%s", e.Code, e.Msg)
}

func New[T ErrCode](code T, msg string) error {
	return &Error{
		Code: RetCode(code),
		Msg:  msg,
	}
}

func Newf[T ErrCode](code T, format string, args ...any) error {
	return &Error{
		Code: RetCode(code),
		Msg:  fmt.Sprintf(format, args...),
	}
}

func Code(err error) RetCode {
	if err == nil {
		return 0
	}
	var e *Error
	if !errors.As(err, &e) {
		return 999
	}
	return e.Code
}

func Msg(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if !errors.As(err, &e) {
		return err.Error()
	}
	return e.Msg
}
