package app

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/canta-9142/qshare/internal/platform/clipboard"
	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/platform/network"
	"github.com/canta-9142/qshare/internal/qr"
	"github.com/canta-9142/qshare/internal/receive"
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

// firewallLease is the application-facing subset of a temporary firewall lease.
type firewallLease interface {
	Close(context.Context) error
}

type Application struct {
	stderr                io.Writer
	stdout                io.Writer
	startShutdownListener func() (<-chan struct{}, func() error, error)
	advertiseEndpoint     func() (network.Endpoint, error)
	selectServerPort      func() (uint16, error)
	openFirewall          func(context.Context, firewall.Rule) (firewallLease, error)
	listen                func(string, string) (net.Listener, error)
	openReceiveStore      func(string) (*receive.Store, error)
	newClipboardSink      func(string) (receive.TextSink, error)
	openPaths             func([]string) (*share.Collection, *share.Directory, error)
	renderQR              func(io.Writer, string) error
}

// Dependencies supplies output streams and optional terminal initialization.
// StartShutdownListener returns the quit notification and terminal restoration.
// On success, Run owns restoration; on failure, initialization cleans up itself.
type Dependencies struct {
	Stdout                io.Writer
	Stderr                io.Writer
	StartShutdownListener func() (<-chan struct{}, func() error, error)
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
		listen:           net.Listen,
		openReceiveStore: receive.OpenStore,
		newClipboardSink: func(backend string) (receive.TextSink, error) {
			sink, err := clipboard.NewSink(backend)
			if errors.Is(err, clipboard.ErrUnsupportedBackend) {
				return nil, invalidRequest(err)
			}
			return sink, err
		},
		openPaths: share.OpenPaths,
		renderQR:  qr.Render,
	}
}
