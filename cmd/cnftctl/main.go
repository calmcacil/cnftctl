package main

import (
	"os"

	"github.com/calmcacil/cnftctl/internal/app"
	"github.com/calmcacil/cnftctl/internal/cli"
)

var version = "dev"

func main() {
	runner := cli.New(cli.Options{Version: version, Service: app.NewService()})
	os.Exit(runner.Run(os.Args[1:]))
}
