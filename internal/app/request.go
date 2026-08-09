package app

import "time"

type NetworkMode int

const (
	NetworkAuto NetworkMode = iota
	NetworkLAN
)

type Request struct {
	Path        string
	NetworkMode NetworkMode
	Lifetime    time.Duration
}
