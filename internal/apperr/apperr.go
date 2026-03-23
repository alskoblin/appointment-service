package apperr

import "fmt"

type Code string

const (
	CodeValidation   Code = "validation_error"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeInternal     Code = "internal_error"
)

type AppError struct {
	Code    Code
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func New(code Code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

func Validation(message string) *AppError {
	return New(CodeValidation, message, nil)
}

func NotFound(message string) *AppError {
	return New(CodeNotFound, message, nil)
}

func Conflict(message string) *AppError {
	return New(CodeConflict, message, nil)
}

func Unauthorized(message string) *AppError {
	return New(CodeUnauthorized, message, nil)
}

func Forbidden(message string) *AppError {
	return New(CodeForbidden, message, nil)
}

func Internal(message string, err error) *AppError {
	return New(CodeInternal, message, err)
}

func As(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}
	appErr, ok := err.(*AppError)
	return appErr, ok
}
