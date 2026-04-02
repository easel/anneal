package main

import (
	"os"

	"github.com/easel/anneal/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr, "dev"))
}
