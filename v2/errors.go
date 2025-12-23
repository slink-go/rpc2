package rpc2

import "encoding/gob"

type CallError struct {
	Message string
}

func NewCallError(message string) *CallError {
	return &CallError{Message: message}
}

func (e *CallError) Error() string {
	return e.Message
}

func init() {
	gob.Register(CallError{})
}
