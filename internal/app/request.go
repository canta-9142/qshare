package app

import "time"

type Operation int

const (
	OperationSend Operation = iota
	OperationReceive
)

type NetworkMode int

const (
	NetworkAuto NetworkMode = iota
	NetworkLAN
)

type Request struct {
	Operation   Operation
	Path        string
	ReceiveDir  string
	NetworkMode NetworkMode
	Lifetime    time.Duration
}
