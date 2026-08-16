package app

import (
	"context"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/qr"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
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
	stdout           io.Writer
	advertiseAddress func() (netip.Addr, error)
	newSendServer    func(*session.Session) sessionServer
	newTextServer    func(*session.Session) sessionServer
	newReceiveServer func(*session.Session, receiveStore, textSubmitter) sessionServer
	openReceiveStore func(string) (receiveStore, error)
	newClipboardSink func(string) (receive.TextSink, error)
	openCollection   func([]string) (*share.Collection, error)
	renderQR         func(io.Writer, string) error
}

type Dependencies struct {
	Stdout io.Writer
	Stderr io.Writer
}

type textSubmitter interface {
	Submit(context.Context, share.Text) error
}

func New(deps Dependencies) *Application {
	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	return &Application{
		stdout:           stdout,
		stderr:           deps.Stderr,
		advertiseAddress: network.AdvertiseAddress,
		newSendServer:    func(s *session.Session) sessionServer { return server.NewSendFile(s) },
		newTextServer:    func(s *session.Session) sessionServer { return server.NewSendText(s) },
		newReceiveServer: func(s *session.Session, store receiveStore, submitter textSubmitter) sessionServer {
			return server.NewReceive(s, store, submitter)
		},
		openReceiveStore: func(dir string) (receiveStore, error) {
			return receive.OpenStore(dir)
		},
		newClipboardSink: func(backend string) (receive.TextSink, error) {
			return clipboard.NewSink(backend)
		},
		openCollection: share.OpenCollection,
		renderQR:       qr.Render,
	}
}
