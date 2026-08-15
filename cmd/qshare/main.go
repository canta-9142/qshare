package main

import (
	"os"

	"github.com/canta-9142/qshare/internal/cli"
)

func main() {
	os.Exit(cli.RunWithStdin(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
