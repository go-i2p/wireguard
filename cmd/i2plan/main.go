// i2plan is a decentralized WireGuard-over-I2P mesh VPN.
//
// It enables fully decentralized private networks without central coordination,
// using I2P for anonymizing transport and WireGuard for the VPN tunnel.
//
// Usage:
//
//	i2plan [flags]
//	i2plan rpc <method> [args]
//
// Flags:
//
//	-config string
//	    Path to configuration file (default "~/.i2plan/config.toml")
//	-name string
//	    Node name (overrides config)
//	-data-dir string
//	    Data directory (overrides config)
//	-sam string
//	    SAM bridge address (overrides config)
//	-v
//	    Enable verbose logging
//	-version
//	    Print version and exit
//
// See https://github.com/go-i2p/wireguard for more information.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/go-i2p/wireguard/lib/core"
)

// Version is set at build time via ldflags.
var Version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// Determine default config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	defaultConfigPath := filepath.Join(homeDir, ".i2plan", "config.toml")

	// Parse command-line flags
	configPath := flag.String("config", defaultConfigPath, "Path to configuration file")
	nodeName := flag.String("name", "", "Node name (overrides config)")
	dataDir := flag.String("data-dir", "", "Data directory (overrides config)")
	samAddr := flag.String("sam", "", "SAM bridge address (overrides config)")
	verbose := flag.Bool("v", false, "Enable verbose logging")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "i2plan - Decentralized WireGuard-over-I2P Mesh VPN\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  i2plan [flags]            Start the node\n")
		fmt.Fprintf(os.Stderr, "  i2plan rpc <method>       Execute RPC method\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("i2plan version %s\n", Version)
		return 0
	}

	// Set up logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Check for RPC subcommand
	args := flag.Args()
	if len(args) > 0 && args[0] == "rpc" {
		return handleRPC(args[1:], logger)
	}

	// Load configuration
	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return 1
	}

	// Apply command-line overrides
	if *nodeName != "" {
		cfg.Node.Name = *nodeName
	}
	if *dataDir != "" {
		cfg.Node.DataDir = *dataDir
	}
	if *samAddr != "" {
		cfg.I2P.SAMAddress = *samAddr
	}

	// Create and start the node
	node, err := core.NewNode(cfg, logger)
	if err != nil {
		logger.Error("failed to create node", "error", err)
		return 1
	}

	// Create a context that is cancelled on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the node
	if err := node.Start(ctx); err != nil {
		logger.Error("failed to start node", "error", err)
		return 1
	}

	logger.Info("i2plan started", "name", cfg.Node.Name, "version", Version)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("received signal, shutting down", "signal", sig)
	case <-node.Done():
		logger.Info("node stopped unexpectedly")
	}

	// Graceful shutdown
	cancel()

	// Create a new context for shutdown with reasonable timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30000000000) // 30 seconds
	defer shutdownCancel()

	if err := node.Stop(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		return 1
	}

	logger.Info("i2plan stopped")
	return 0
}

// handleRPC handles the "rpc" subcommand.
// TODO (Phase 4): Implement RPC client
func handleRPC(args []string, logger *slog.Logger) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: i2plan rpc <method> [args...]")
		fmt.Fprintln(os.Stderr, "\nAvailable methods:")
		fmt.Fprintln(os.Stderr, "  status          Show node status")
		fmt.Fprintln(os.Stderr, "  peers.list      List known peers")
		fmt.Fprintln(os.Stderr, "  invite.create   Generate invite code")
		fmt.Fprintln(os.Stderr, "  invite.accept   Accept invite code")
		return 1
	}

	method := args[0]
	logger.Debug("RPC method called", "method", method, "args", args[1:])

	// TODO: Connect to RPC socket and execute method
	fmt.Fprintf(os.Stderr, "RPC not yet implemented (method: %s)\n", method)
	return 1
}
