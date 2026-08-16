package app

import (
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

type Operation int

const (
	OperationSendFile Operation = iota
	OperationSendDirectory
	OperationSendText
	OperationReceive
)

type NetworkMode int

const (
	NetworkAuto NetworkMode = iota
	NetworkLAN
)

type Request struct {
	Operation   Operation
	Paths       []string
	Text        share.Text
	ReceiveDir  string
	Clipboard   string
	NetworkMode NetworkMode
	Lifetime    time.Duration
}
