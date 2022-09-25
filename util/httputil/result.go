package httputil

import (
	"encoding/json"
	"net/http"
)

const ResultCodeSuccess = 0
const ResultCodePluginNotInstall = 1
const ResultCodeFailed = 2
const ResultCodeError = 3

type Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Body    T      `json:"body"`
}

func Success[T any](body T) Result[T] {
	return Result[T]{
		Code:    ResultCodeSuccess,
		Message: "",
		Body:    body,
	}
}

func Failed(message string) Result[interface{}] {
	return Result[interface{}]{
		Code:    ResultCodeFailed,
		Message: message,
	}
}

func Error(err error) Result[interface{}] {
	return Result[interface{}]{
		Code:    ResultCodeError,
		Message: err.Error(),
	}
}

func OutResult[T any](w http.ResponseWriter, result Result[T]) error {
	w.WriteHeader(200)
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	if err != nil {
		return err
	}
	return nil
}

func New[T any](code int, message string, body T) Result[T] {
	return Result[T]{
		code,
		message,
		body,
	}
}
