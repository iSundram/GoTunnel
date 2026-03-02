package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/iSundram/GoTunnel/internal/client"
	"github.com/iSundram/GoTunnel/internal/config"
	"github.com/iSundram/GoTunnel/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "server":
		runServer(os.Args[2:])
	case "client":
		runClient(os.Args[2:])
	case "token":
		fmt.Fprintln(os.Stderr, "token management is not implemented yet")
		os.Exit(1)
	case "tunnel":
		fmt.Fprintln(os.Stderr, "tunnel management is not implemented yet")
		os.Exit(1)
	case "version":
		fmt.Println("GoTunnel v0.1.0")
	default:
		printUsage()
		os.Exit(0)
	}
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", "/etc/gotunnel/config.yml", "path to config file")
	logLevel := fs.String("log-level", "", "override log level (info|warn|debug|error)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating server: %v\n", err)
		os.Exit(1)
	}

	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
}

func runClient(args []string) {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	configPath := fs.String("config", "~/.gotunnel/client.yml", "path to config file")
	logLevel := fs.String("log-level", "", "override log level (info|warn|debug|error)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_ = logLevel // client config does not currently expose a logging field

	cfg, err := config.LoadClientConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading client config: %v\n", err)
		os.Exit(1)
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating client: %v\n", err)
		os.Exit(1)
	}

	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running client: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: gotunnel <command> [flags]

Commands:
  server   Start the GoTunnel server
  client   Start the GoTunnel client
  token    Manage API tokens
  tunnel   Manage active tunnels
  version  Print version information

Flags:
  --config     Path to config file
  --log-level  Override log level (info|warn|debug|error)`)
}
