package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/jeronimoalbi/tail-ws/broadcast"
)

const usage = `Usage: tail-ws [OPTION]... FILE

WebSocket broadcaster for appended file lines.

Options:`

func main() {
	var (
		addr, origin, certFile, keyFile string
		verbose                         bool
	)

	flag.StringVar(&addr, "address", "127.0.0.1:8080", "server address")
	flag.StringVar(&origin, "allow-origin", "", "address of the origin allowed to connect")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.StringVar(&certFile, "cert-file", "", "certificate file for WSS server")
	flag.StringVar(&keyFile, "key-file", "", "private key file for WSS server")
	flag.Parse()

	fileName := flag.Arg(0)
	if fileName == "" {
		printUsage()
		os.Exit(1)
	}

	if !verbose {
		log.SetOutput(io.Discard)
	}

	server := broadcast.NewServer(
		broadcast.Address(addr),
		broadcast.Origin(origin),
		broadcast.Secure(certFile, keyFile),
	)
	g, ctx := errgroup.WithContext(context.Background())

	g.Go(func() error {
		return server.Watch(ctx, fileName)
	})

	g.Go(func() error {
		return server.Start(ctx)
	})

	if err := g.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, usage)
	flag.PrintDefaults()
}
