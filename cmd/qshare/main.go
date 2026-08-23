package main

import (
	"os"

	"github.com/canta-9142/qshare/internal/cli"
	"github.com/canta-9142/qshare/internal/platform/firewall"
)

func main() {
	if handled, code := firewall.RunHelperIfRequested(
		os.Args[1:],
		os.Stdin,
		os.Stdout,
		os.Stderr,
	); handled {
		os.Exit(code)
	}
	os.Exit(cli.RunWithStdin(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
