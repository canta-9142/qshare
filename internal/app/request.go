package app

import (
	"errors"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

var ErrInvalidRequest = errors.New("invalid application request")

type invalidRequestError struct {
	err error
}

func (e invalidRequestError) Error() string {
	return e.err.Error()
}

func (e invalidRequestError) Unwrap() error {
	return e.err
}

func (e invalidRequestError) Is(target error) bool {
	return target == ErrInvalidRequest
}

func invalidRequest(err error) error {
	return invalidRequestError{err: err}
}

type Operation int

const (
	OperationSendFile Operation = iota
	OperationSendDirectory
	OperationSendText
	OperationReceive
)

type Request struct {
	Operation  Operation
	Paths      []string
	Text       share.Text
	ReceiveDir string
	Clipboard  string
	Lifetime   time.Duration
}
