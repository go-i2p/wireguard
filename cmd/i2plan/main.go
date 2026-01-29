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
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/go-i2p/wireguard/lib/core"
	"github.com/go-i2p/wireguard/lib/rpc"
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
		// Determine data directory for RPC socket
		rpcDataDir := *dataDir
		if rpcDataDir == "" {
			rpcDataDir = filepath.Join(homeDir, ".i2plan")
		}
		return handleRPC(args[1:], logger, rpcDataDir)
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
func handleRPC(args []string, _ *slog.Logger, dataDir string) int {
	if len(args) == 0 {
		printRPCUsage()
		return 1
	}

	method := args[0]
	methodArgs := args[1:]

	// Determine socket path
	socketPath := filepath.Join(dataDir, core.DefaultRPCSocket)
	if envSocket := os.Getenv("I2PLAN_RPC_SOCKET"); envSocket != "" {
		socketPath = envSocket
	}

	// Determine auth file
	authFile := filepath.Join(dataDir, "rpc_auth.token")
	if envAuth := os.Getenv("I2PLAN_RPC_AUTH"); envAuth != "" {
		authFile = envAuth
	}

	// Create RPC client
	client, err := rpc.NewClient(rpc.ClientConfig{
		UnixSocketPath: socketPath,
		AuthFile:       authFile,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to RPC: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the i2plan daemon running?\n")
		return 1
	}
	defer client.Close()

	ctx := context.Background()

	switch method {
	case "status":
		return rpcStatus(ctx, client)
	case "peers.list":
		return rpcPeersList(ctx, client)
	case "peers.connect":
		return rpcPeersConnect(ctx, client, methodArgs)
	case "invite.create":
		return rpcInviteCreate(ctx, client, methodArgs)
	case "invite.accept":
		return rpcInviteAccept(ctx, client, methodArgs)
	case "routes.list":
		return rpcRoutesList(ctx, client)
	case "config.get":
		return rpcConfigGet(ctx, client, methodArgs)
	case "config.set":
		return rpcConfigSet(ctx, client, methodArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown method: %s\n\n", method)
		printRPCUsage()
		return 1
	}
}

func printRPCUsage() {
	fmt.Fprintln(os.Stderr, "Usage: i2plan rpc <method> [args...]")
	fmt.Fprintln(os.Stderr, "\nAvailable methods:")
	fmt.Fprintln(os.Stderr, "  status              Show node status")
	fmt.Fprintln(os.Stderr, "  peers.list          List known peers")
	fmt.Fprintln(os.Stderr, "  peers.connect CODE  Connect using invite code")
	fmt.Fprintln(os.Stderr, "  invite.create       Generate invite code")
	fmt.Fprintln(os.Stderr, "  invite.accept CODE  Accept invite code")
	fmt.Fprintln(os.Stderr, "  routes.list         Show routing table")
	fmt.Fprintln(os.Stderr, "  config.get [KEY]    Get configuration")
	fmt.Fprintln(os.Stderr, "  config.set KEY VAL  Set configuration")
}

func rpcStatus(ctx context.Context, client *rpc.Client) int {
	result, err := client.Status(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Node Name:    %s\n", result.NodeName)
	fmt.Printf("Node ID:      %s\n", result.NodeID)
	fmt.Printf("State:        %s\n", result.State)
	if result.TunnelIP != "" {
		fmt.Printf("Tunnel IP:    %s\n", result.TunnelIP)
	}
	if result.I2PDestination != "" {
		fmt.Printf("I2P Dest:     %s\n", result.I2PDestination)
	}
	fmt.Printf("Peers:        %d\n", result.PeerCount)
	fmt.Printf("Uptime:       %s\n", result.Uptime)
	fmt.Printf("Version:      %s\n", result.Version)

	return 0
}

func rpcPeersList(ctx context.Context, client *rpc.Client) int {
	result, err := client.PeersList(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if result.Total == 0 {
		fmt.Println("No peers")
		return 0
	}

	fmt.Printf("%-16s %-16s %-12s %-20s\n", "NODE ID", "TUNNEL IP", "STATE", "LAST SEEN")
	fmt.Printf("%-16s %-16s %-12s %-20s\n", "-------", "---------", "-----", "---------")
	for _, peer := range result.Peers {
		nodeID := peer.NodeID
		if len(nodeID) > 16 {
			nodeID = nodeID[:13] + "..."
		}
		fmt.Printf("%-16s %-16s %-12s %-20s\n", nodeID, peer.TunnelIP, peer.State, peer.LastSeen)
	}
	fmt.Printf("\nTotal: %d peers\n", result.Total)

	return 0
}

func rpcPeersConnect(ctx context.Context, client *rpc.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: i2plan rpc peers.connect <invite_code>")
		return 1
	}

	result, err := client.PeersConnect(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Connected to peer: %s\n", result.NodeID)
	fmt.Printf("Tunnel IP: %s\n", result.TunnelIP)
	if result.Message != "" {
		fmt.Printf("Message: %s\n", result.Message)
	}

	return 0
}

func rpcInviteCreate(ctx context.Context, client *rpc.Client, args []string) int {
	expiry := "24h"
	maxUses := 1

	// Parse optional args
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-expiry":
			if i+1 < len(args) {
				expiry = args[i+1]
				i++
			}
		case "-uses":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxUses)
				i++
			}
		}
	}

	result, err := client.InviteCreate(ctx, expiry, maxUses)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Invite Code:\n%s\n\n", result.InviteCode)
	fmt.Printf("Expires: %s\n", result.ExpiresAt)
	fmt.Printf("Max Uses: %d\n", result.MaxUses)

	return 0
}

func rpcInviteAccept(ctx context.Context, client *rpc.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: i2plan rpc invite.accept <invite_code>")
		return 1
	}

	result, err := client.InviteAccept(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Joined network: %s\n", result.NetworkID)
	fmt.Printf("Connected to: %s\n", result.PeerNodeID)
	fmt.Printf("Tunnel IP: %s\n", result.TunnelIP)
	if result.Message != "" {
		fmt.Printf("Message: %s\n", result.Message)
	}

	return 0
}

func rpcRoutesList(ctx context.Context, client *rpc.Client) int {
	result, err := client.RoutesList(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if result.Total == 0 {
		fmt.Println("No routes")
		return 0
	}

	fmt.Printf("%-16s %-16s %-6s %-16s %-20s\n", "TUNNEL IP", "NODE ID", "HOPS", "VIA", "LAST SEEN")
	fmt.Printf("%-16s %-16s %-6s %-16s %-20s\n", "---------", "-------", "----", "---", "---------")
	for _, route := range result.Routes {
		nodeID := route.NodeID
		if len(nodeID) > 16 {
			nodeID = nodeID[:13] + "..."
		}
		via := route.ViaNodeID
		if via == "" {
			via = "(direct)"
		} else if len(via) > 16 {
			via = via[:13] + "..."
		}
		fmt.Printf("%-16s %-16s %-6d %-16s %-20s\n", route.TunnelIP, nodeID, route.HopCount, via, route.LastSeen)
	}
	fmt.Printf("\nTotal: %d routes\n", result.Total)

	return 0
}

func rpcConfigGet(ctx context.Context, client *rpc.Client, args []string) int {
	key := ""
	if len(args) > 0 {
		key = args[0]
	}

	result, err := client.ConfigGet(ctx, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if key == "" {
		// Pretty print entire config
		data, err := json.MarshalIndent(result.Value, "", "  ")
		if err != nil {
			fmt.Printf("%v\n", result.Value)
		} else {
			fmt.Println(string(data))
		}
	} else {
		fmt.Printf("%s = %v\n", key, result.Value)
	}

	return 0
}

func rpcConfigSet(ctx context.Context, client *rpc.Client, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: i2plan rpc config.set <key> <value>")
		return 1
	}

	key := args[0]
	value := args[1]

	// Try to parse as JSON first, otherwise use as string
	var parsedValue any
	if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
		parsedValue = value
	}

	result, err := client.ConfigSet(ctx, key, parsedValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Set %s: %v -> %v\n", result.Key, result.OldValue, result.NewValue)
	return 0
}
