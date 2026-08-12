package cli

import (
	"errors"
	"fmt"
	"io"

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
	if len(args.Files) != 1 {
		return parseResult{}, errors.New("exactly one file must be specified")
	}
	if args.Expire <= 0 {
		return parseResult{}, errors.New("session lifetime must be greater than zero")
	}

	mode := app.NetworkAuto
	if args.LAN {
		mode = app.NetworkLAN
	}

	return parseResult{
		Request: app.Request{
			Path:        args.Files[0],
			NetworkMode: mode,
			Lifetime:    args.Expire,
		},
	}, nil
}
