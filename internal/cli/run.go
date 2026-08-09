package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/canta-9142/qshare/internal/app"
)

func Run(argv []string, stdout io.Writer, stderr io.Writer) int {
	result, err := parse(argv, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		return 2
	}

	if result.Exit {
		return result.Code
	}

	ctx := context.Background()

	application := app.New(app.Dependencies{
		Stderr: stderr,
	})

	if err := application.Run(ctx, result.Request); err != nil {
		fmt.Fprintf(stderr, "qshare: %v\n", err)
		return 1
	}

	return 0
}
