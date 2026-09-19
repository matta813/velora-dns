package main

import (
	"flag"
	"os"
	"testing"
)

func TestRunRejectsInvalidHealthcheckURL(t *testing.T) {
	originalArgs, originalFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = originalArgs, originalFlags })
	os.Args = []string{"velora-dns", "-healthcheck", "://invalid"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	if err := run(); err == nil {
		t.Fatal("invalid healthcheck URL was accepted")
	}
}
