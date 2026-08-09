package main

import (
	"os"

	"github.com/canta-9142/qshare/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
