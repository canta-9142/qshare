package app

import (
	"context"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/qr"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
)

const (
	defaultServerPort      = "55544"
	expirationDrainTimeout = 30 * time.Second
)

type shutdownServer interface {
	Shutdown(context.Context) error
	Close() error
}

type sessionServer interface {
	shutdownServer
	Start(string) (net.Addr, error)
	Done() <-chan error
}

type receiveStore interface {
	Save(context.Context, string, io.Reader) (receive.Result, error)
}

type Application struct {
	stderr           io.Writer
	advertiseAddress func() (netip.Addr, error)
	newSendServer    func(*session.Session) sessionServer
	newReceiveServer func(*session.Session, receiveStore) sessionServer
	openReceiveStore func(string) (receiveStore, error)
	renderQR         func(io.Writer, string) error
}

type Dependencies struct {
	Stderr io.Writer
}

func New(deps Dependencies) *Application {
	return &Application{
		stderr:           deps.Stderr,
		advertiseAddress: network.AdvertiseAddress,
		newSendServer:    func(s *session.Session) sessionServer { return server.NewSend(s) },
		newReceiveServer: func(s *session.Session, store receiveStore) sessionServer {
			return server.NewReceive(s, store)
		},
		openReceiveStore: func(dir string) (receiveStore, error) {
			return receive.OpenStore(dir)
		},
		renderQR: qr.Render,
	}
}
