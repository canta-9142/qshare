package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alexflint/go-arg"
	"github.com/canta-9142/qshare/internal/app"
	"github.com/canta-9142/qshare/internal/share"
)

type parseResult struct {
	Request app.Request
	Exit    bool
	Code    int
}

func parse(argv []string, stdout io.Writer, stderr io.Writer) (parseResult, error) {
	return parseWithInput(argv, stdinInput{terminal: true}, stdout, stderr)
}

type stdinInput struct {
	reader   io.Reader
	terminal bool
}

func parseWithInput(argv []string, stdin stdinInput, stdout io.Writer, stderr io.Writer) (parseResult, error) {
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

	return mapArgumentsWithInput(args, stdin)
}

func mapArguments(args arguments) (parseResult, error) {
	return mapArgumentsWithInput(args, stdinInput{terminal: true})
}

func mapArgumentsWithInput(args arguments, stdin stdinInput) (parseResult, error) {
	if args.Expire <= 0 {
		return parseResult{}, errors.New("session lifetime must be greater than zero")
	}
	if len(args.Files) > share.MaxFiles {
		return parseResult{}, fmt.Errorf("too many files: got %d, maximum is %d", len(args.Files), share.MaxFiles)
	}

	networkMode := app.NetworkAuto
	if args.LAN {
		networkMode = app.NetworkLAN
	}

	if !stdin.terminal {
		switch {
		case len(args.Files) != 0:
			return parseResult{}, errors.New("piped stdin cannot be combined with a file")
		case args.Text != nil:
			return parseResult{}, errors.New("piped stdin cannot be combined with --text")
		case args.Clipboard != nil:
			return parseResult{}, errors.New("piped stdin cannot be combined with --clipboard")
		case args.ReceiveDir != "":
			return parseResult{}, errors.New("piped stdin cannot be combined with --receive-dir")
		}

		value, err := io.ReadAll(io.LimitReader(stdin.reader, share.MaxTextSize+1))
		if err != nil {
			return parseResult{}, fmt.Errorf("read text from stdin: %w", err)
		}
		text, err := share.NewText(value)
		if err != nil {
			return parseResult{}, fmt.Errorf("invalid stdin text: %w", err)
		}

		return parseResult{
			Request: app.Request{
				Operation:   app.OperationSendText,
				Text:        text,
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil
	}

	if args.Text != nil {
		if len(args.Files) != 0 {
			return parseResult{}, errors.New("--text cannot be combined with a file")
		}
		if args.ReceiveDir != "" {
			return parseResult{}, errors.New("--receive-dir cannot be used when sharing text")
		}
		if args.Clipboard != nil {
			return parseResult{}, errors.New("--text cannot be combined with --clipboard")
		}

		text, err := share.NewText([]byte(*args.Text))
		if err != nil {
			return parseResult{}, fmt.Errorf("invalid --text value: %w", err)
		}

		return parseResult{
			Request: app.Request{
				Operation:   app.OperationSendText,
				Text:        text,
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil
	}

	if args.Clipboard != nil {
		if len(args.Files) != 0 {
			return parseResult{}, errors.New("--clipboard cannot be combined with a file")
		}
		if *args.Clipboard == "" {
			return parseResult{}, errors.New("--clipboard requires a non-empty backend")
		}

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
				Clipboard:   *args.Clipboard,
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil
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
				Clipboard:   "auto",
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil

	default:
		if args.ReceiveDir != "" {
			return parseResult{}, errors.New("--receive-dir cannot be used when sharing a file")
		}

		operation, err := classifySendPaths(args.Files)
		if err != nil {
			return parseResult{}, err
		}
		return parseResult{
			Request: app.Request{
				Operation:   operation,
				Paths:       append([]string(nil), args.Files...),
				NetworkMode: networkMode,
				Lifetime:    args.Expire,
			},
		}, nil

	}
}

func classifySendPaths(paths []string) (app.Operation, error) {
	hasDirectory := false
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err == nil && info.IsDir() {
			hasDirectory = true
		}
	}
	if hasDirectory {
		if len(paths) != 1 {
			return 0, errors.New("a directory cannot be combined with another path")
		}
		return app.OperationSendDirectory, nil
	}
	return app.OperationSendFile, nil
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
