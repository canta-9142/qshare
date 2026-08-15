package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alexflint/go-arg"
	"github.com/canta-9142/qshare/internal/app"
)

type parseResult struct {
	Request app.Request
	Exit    bool
	Code    int
}

func parse(argv []string, stdout io.Writer, stderr io.Writer) (parseResult, error) {
	var args arguments

	parser, err := arg.NewParser(arg.Config{
		Program: "qshare",
	}, &args)
	if err != nil {
		return parseResult{}, fmt.Errorf("create argument parser: %w", err)
	}

	err = parser.Parse(argv)

	switch {
	case errors.Is(err, arg.ErrHelp):
		parser.WriteHelp(stdout)
		return parseResult{
			Exit: true,
			Code: 0,
		}, nil

	case err != nil:
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		parser.WriteUsage(stderr)
		return parseResult{
			Exit: true,
			Code: 2,
		}, nil
	}

	return mapArguments(args)
}

func mapArguments(args arguments) (parseResult, error) {
	if args.Expire <= 0 {
		return parseResult{}, errors.New("session lifetime must be greater than zero")
	}

	networkMode := app.NetworkAuto
	if args.LAN {
		networkMode = app.NetworkLAN
	}

	switch len(args.Files) {
	case 0:
		receiveDir := args.ReceiveDir
		if receiveDir == "" {
			dir, err := defaultReceiveDir()
			if err != nil {
				return parseResult{}, err
			}
			receiveDir = dir
		}

		return parseResult{
			Request: app.Request{
				Operation:   app.OperationReceive,
				ReceiveDir:  receiveDir,
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil

	case 1:
		if args.ReceiveDir != "" {
			return parseResult{}, errors.New("--receive-dir cannot be used when sharing a file")
		}

		return parseResult{
			Request: app.Request{
				Operation:   app.OperationSend,
				Path:        args.Files[0],
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil

	default:
		return parseResult{}, errors.New("at most one file may be specified")
	}
}

func defaultReceiveDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine user home directory: %w", err)
	}

	return receiveDirFromHome(home), nil
}

func receiveDirFromHome(home string) string {
	return filepath.Join(home, "Downloads", "qshare")
}
