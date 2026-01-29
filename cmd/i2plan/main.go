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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-i2p/wireguard/lib/core"
	"github.com/go-i2p/wireguard/lib/embedded"
	"github.com/go-i2p/wireguard/lib/rpc"
	"github.com/go-i2p/wireguard/lib/tui"
	"github.com/go-i2p/wireguard/lib/web"
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
		fmt.Fprintf(os.Stderr, "  i2plan rpc <method>       Execute RPC method\n")
		fmt.Fprintf(os.Stderr, "  i2plan tui                Launch interactive TUI\n")
		fmt.Fprintf(os.Stderr, "  i2plan web                Start web UI server\n\n")
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

	// Check for TUI subcommand
	if len(args) > 0 && args[0] == "tui" {
		// Determine data directory for RPC socket
		tuiDataDir := *dataDir
		if tuiDataDir == "" {
			tuiDataDir = filepath.Join(homeDir, ".i2plan")
		}
		return handleTUI(logger, tuiDataDir)
	}

	// Check for Web subcommand
	if len(args) > 0 && args[0] == "web" {
		// Determine data directory for RPC socket
		webDataDir := *dataDir
		if webDataDir == "" {
			webDataDir = filepath.Join(homeDir, ".i2plan")
		}
		return handleWeb(logger, webDataDir)
	}

	// Build embedded VPN configuration
	// Start with defaults, then apply config file, then CLI overrides
	vpnConfig := embedded.DefaultConfig()
	vpnConfig.Logger = logger

	// Load configuration file for additional settings
	cfg, err := core.LoadConfig(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return 1
	}

	// Apply config file values
	vpnConfig.NodeName = cfg.Node.Name
	vpnConfig.DataDir = cfg.Node.DataDir
	vpnConfig.SAMAddress = cfg.I2P.SAMAddress
	vpnConfig.TunnelLength = cfg.I2P.TunnelLength
	vpnConfig.TunnelSubnet = cfg.Mesh.TunnelSubnet
	vpnConfig.MaxPeers = cfg.Mesh.MaxPeers
	vpnConfig.EnableRPC = cfg.RPC.Enabled
	vpnConfig.RPCSocket = cfg.RPC.Socket
	vpnConfig.EnableWeb = cfg.Web.Enabled
	vpnConfig.WebListenAddr = cfg.Web.Listen

	// Apply command-line overrides
	if *nodeName != "" {
		vpnConfig.NodeName = *nodeName
	}
	if *dataDir != "" {
		vpnConfig.DataDir = *dataDir
	}
	if *samAddr != "" {
		vpnConfig.SAMAddress = *samAddr
	}

	// Create the embedded VPN
	vpn, err := embedded.New(vpnConfig)
	if err != nil {
		logger.Error("failed to create VPN", "error", err)
		return 1
	}
	defer vpn.Close()

	// Create a context that is cancelled on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the VPN
	if err := vpn.Start(ctx); err != nil {
		logger.Error("failed to start VPN", "error", err)
		return 1
	}

	logger.Info("i2plan started", "name", vpnConfig.NodeName, "version", Version)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("received signal, shutting down", "signal", sig)
	case <-vpn.Done():
		logger.Info("VPN stopped unexpectedly")
	}

	// Graceful shutdown
	cancel()

	// Create a new context for shutdown with reasonable timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := vpn.Stop(shutdownCtx); err != nil {
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

// handleTUI launches the interactive terminal user interface.
func handleTUI(logger *slog.Logger, dataDir string) int {
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

	// Create TUI
	tuiApp, err := tui.New(tui.Config{
		RPCSocketPath:   socketPath,
		RPCAuthFile:     authFile,
		RefreshInterval: 5 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the i2plan daemon running?\n")
		return 1
	}
	defer tuiApp.Close()

	// Run the TUI
	p := tea.NewProgram(tuiApp, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		return 1
	}

	return 0
}

// handleWeb starts the web UI server.
func handleWeb(logger *slog.Logger, dataDir string) int {
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

	// Default listen address
	listenAddr := "127.0.0.1:8080"
	if envAddr := os.Getenv("I2PLAN_WEB_ADDR"); envAddr != "" {
		listenAddr = envAddr
	}

	// Create web server
	server, err := web.New(web.Config{
		ListenAddr:    listenAddr,
		RPCSocketPath: socketPath,
		RPCAuthFile:   authFile,
		Logger:        logger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the i2plan daemon running?\n")
		return 1
	}

	// Start the server
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting web server: %v\n", err)
		return 1
	}

	fmt.Printf("Web UI running at http://%s\n", listenAddr)
	fmt.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping web server: %v\n", err)
		return 1
	}

	return 0
}
