package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/qr"
	"github.com/canta-9142/qshare/internal/receive"
	"github.com/canta-9142/qshare/internal/server"
	"github.com/canta-9142/qshare/internal/session"
	"github.com/canta-9142/qshare/internal/share"
)

// Server port and shutdown constants define the LAN session lifecycle bounds.
const (
	minimumServerPort      = 50000
	serverPortCount        = 10000
	serverPortAttempts     = 32
	expirationDrainTimeout = 30 * time.Second
	firewallCleanupTimeout = 5 * time.Second
	firewallTimeoutSlack   = 5 * time.Second
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

// firewallLease is the application-facing subset of a temporary firewall lease.
type firewallLease interface {
	Close(context.Context) error
}

type Application struct {
	stderr                io.Writer
	stdout                io.Writer
	startShutdownListener func() (<-chan struct{}, error)
	shutdownRequested     <-chan struct{}
	advertiseEndpoint     func() (network.Endpoint, error)
	selectServerPort      func() (uint16, error)
	openFirewall          func(context.Context, firewall.Rule) (firewallLease, error)
	newSendServer         func(*session.Session) sessionServer
	newDirectoryServer    func(*session.Session) sessionServer
	newTextServer         func(*session.Session) sessionServer
	newReceiveServer      func(*session.Session, receiveStore, textSubmitter) sessionServer
	openReceiveStore      func(string) (receiveStore, error)
	newClipboardSink      func(string) (receive.TextSink, error)
	openCollection        func([]string) (*share.Collection, error)
	openDirectory         func(string) (*share.Directory, error)
	renderQR              func(io.Writer, string) error
}

type Dependencies struct {
	Stdout                io.Writer
	Stderr                io.Writer
	StartShutdownListener func() (<-chan struct{}, error)
}

type textSubmitter interface {
	Submit(context.Context, share.Text) error
}

// randomServerPort selects a uniformly distributed port from the configured range.
func randomServerPort() (uint16, error) {
	offset, err := rand.Int(rand.Reader, big.NewInt(serverPortCount))
	if err != nil {
		return 0, fmt.Errorf("select random server port: %w", err)
	}
	return uint16(minimumServerPort + offset.Int64()), nil
}

func New(deps Dependencies) *Application {
	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	return &Application{
		stdout:                stdout,
		stderr:                deps.Stderr,
		startShutdownListener: deps.StartShutdownListener,
		advertiseEndpoint:     network.AdvertiseEndpoint,
		selectServerPort:      randomServerPort,
		openFirewall: func(ctx context.Context, rule firewall.Rule) (firewallLease, error) {
			return firewall.Open(ctx, rule)
		},
		newSendServer:      func(s *session.Session) sessionServer { return server.NewSendFile(s) },
		newDirectoryServer: func(s *session.Session) sessionServer { return server.NewSendDirectory(s) },
		newTextServer:      func(s *session.Session) sessionServer { return server.NewSendText(s) },
		newReceiveServer: func(s *session.Session, store receiveStore, submitter textSubmitter) sessionServer {
			return server.NewReceive(s, store, submitter)
		},
		openReceiveStore: func(dir string) (receiveStore, error) {
			return receive.OpenStore(dir)
		},
		newClipboardSink: func(backend string) (receive.TextSink, error) {
			sink, err := clipboard.NewSink(backend)
			if errors.Is(err, clipboard.ErrUnsupportedBackend) {
				return nil, invalidRequest(err)
			}
			return sink, err
		},
		openCollection: share.OpenCollection,
		openDirectory:  share.OpenDirectory,
		renderQR:       qr.Render,
	}
}
