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

// cliFlags holds parsed command-line flags.
type cliFlags struct {
	configPath  string
	nodeName    string
	dataDir     string
	samAddr     string
	verbose     bool
	showVersion bool
	homeDir     string
}

// parseFlags parses command-line flags and returns the configuration.
func parseFlags() *cliFlags {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	defaultConfigPath := filepath.Join(homeDir, ".i2plan", "config.toml")

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

	return &cliFlags{
		configPath:  *configPath,
		nodeName:    *nodeName,
		dataDir:     *dataDir,
		samAddr:     *samAddr,
		verbose:     *verbose,
		showVersion: *showVersion,
		homeDir:     homeDir,
	}
}

// enableDebugLogging sets up debug logging if verbose mode is enabled.
func enableDebugLogging(verbose bool) {
	if verbose {
		os.Setenv("DEBUG_I2P", "debug")
	}
}

// handleSubcommand checks for and handles subcommands (rpc, tui, web).
// Returns exit code and true if a subcommand was handled, or 0 and false otherwise.
func handleSubcommand(flags *cliFlags) (int, bool) {
	args := flag.Args()
	if len(args) == 0 {
		return 0, false
	}

	dataDir := flags.dataDir
	if dataDir == "" {
		dataDir = filepath.Join(flags.homeDir, ".i2plan")
	}

	switch args[0] {
	case "rpc":
		return handleRPC(args[1:], dataDir), true
	case "tui":
		return handleTUI(dataDir), true
	case "web":
		return handleWeb(dataDir), true
	default:
		return 0, false
	}
}

// buildVPNConfig creates the VPN configuration from config file and CLI flags.
func buildVPNConfig(flags *cliFlags) (*embedded.Config, error) {
	vpnConfig := embedded.DefaultConfig()

	cfg, err := core.LoadConfig(flags.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

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

	if flags.nodeName != "" {
		vpnConfig.NodeName = flags.nodeName
	}
	if flags.dataDir != "" {
		vpnConfig.DataDir = flags.dataDir
	}
	if flags.samAddr != "" {
		vpnConfig.SAMAddress = flags.samAddr
	}

	return &vpnConfig, nil
}

// runVPN starts the VPN and waits for shutdown signal.
func runVPN(vpnConfig *embedded.Config) int {
	vpn, err := embedded.New(*vpnConfig)
	if err != nil {
		log.WithError(err).Error("failed to create VPN")
		return 1
	}
	defer vpn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := vpn.Start(ctx); err != nil {
		log.WithError(err).Error("failed to start VPN")
		return 1
	}

	log.WithField("name", vpnConfig.NodeName).WithField("version", Version).Info("i2plan started")

	select {
	case sig := <-sigChan:
		log.WithField("signal", sig).Info("received signal, shutting down")
	case <-vpn.Done():
		log.Info("VPN stopped unexpectedly")
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := vpn.Stop(shutdownCtx); err != nil {
		log.WithError(err).Error("shutdown error")
		return 1
	}

	log.Info("i2plan stopped")
	return 0
}

func run() int {
	flags := parseFlags()

	if flags.showVersion {
		fmt.Printf("i2plan version %s\n", Version)
		return 0
	}

	enableDebugLogging(flags.verbose)

	if exitCode, handled := handleSubcommand(flags); handled {
		return exitCode
	}

	vpnConfig, err := buildVPNConfig(flags)
	if err != nil {
		log.WithError(err).Error("configuration error")
		return 1
	}

	return runVPN(vpnConfig)
}

// handleRPC handles the "rpc" subcommand.
func handleRPC(args []string, dataDir string) int {
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
	authFile := filepath.Join(dataDir, "rpc.auth")
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
	case "bans.list":
		return rpcBansList(ctx, client)
	case "bans.add":
		return rpcBansAdd(ctx, client, methodArgs)
	case "bans.remove":
		return rpcBansRemove(ctx, client, methodArgs)
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
	fmt.Fprintln(os.Stderr, "  bans.list           List banned peers")
	fmt.Fprintln(os.Stderr, "  bans.add ID REASON  Ban a peer")
	fmt.Fprintln(os.Stderr, "  bans.remove ID      Unban a peer")
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

	printRoutesHeader()
	for _, route := range result.Routes {
		printRouteEntry(route)
	}
	fmt.Printf("\nTotal: %d routes\n", result.Total)

	return 0
}

// printRoutesHeader prints the header row for routes table.
func printRoutesHeader() {
	fmt.Printf("%-16s %-16s %-6s %-16s %-20s\n", "TUNNEL IP", "NODE ID", "HOPS", "VIA", "LAST SEEN")
	fmt.Printf("%-16s %-16s %-6s %-16s %-20s\n", "---------", "-------", "----", "---", "---------")
}

// printRouteEntry prints a single route entry formatted for display.
func printRouteEntry(route rpc.RouteInfo) {
	nodeID := truncateID(route.NodeID, 16)
	via := formatViaNode(route.ViaNodeID)
	fmt.Printf("%-16s %-16s %-6d %-16s %-20s\n", route.TunnelIP, nodeID, route.HopCount, via, route.LastSeen)
}

// truncateID shortens an ID to the specified length for display.
func truncateID(id string, maxLen int) string {
	if len(id) > maxLen {
		return id[:maxLen-3] + "..."
	}
	return id
}

// formatViaNode formats the via node ID for display.
func formatViaNode(viaNodeID string) string {
	if viaNodeID == "" {
		return "(direct)"
	}
	return truncateID(viaNodeID, 16)
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

func rpcBansList(ctx context.Context, client *rpc.Client) int {
	result, err := client.BansList(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(result.Bans) == 0 {
		fmt.Println("No banned peers")
		return 0
	}

	fmt.Printf("Banned Peers (%d):\n", len(result.Bans))
	for _, ban := range result.Bans {
		fmt.Printf("  %s\n", ban.NodeID)
		fmt.Printf("    Reason: %s\n", ban.Reason)
		if ban.Description != "" {
			fmt.Printf("    Description: %s\n", ban.Description)
		}
		fmt.Printf("    Banned: %s\n", ban.BannedAt.Format(time.RFC3339))
		if ban.ExpiresAt != nil && !ban.ExpiresAt.IsZero() {
			fmt.Printf("    Expires: %s\n", ban.ExpiresAt.Format(time.RFC3339))
		}
	}
	return 0
}

func rpcBansAdd(ctx context.Context, client *rpc.Client, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: i2plan rpc bans.add <node_id> <reason> [description] [duration]")
		return 1
	}

	nodeID := args[0]
	reason := args[1]
	description := ""
	duration := "0" // permanent

	if len(args) > 2 {
		description = args[2]
	}
	if len(args) > 3 {
		duration = args[3]
	}

	result, err := client.BansAdd(ctx, nodeID, reason, description, duration)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if result.Success {
		fmt.Printf("Banned peer: %s\n", nodeID)
	} else {
		fmt.Printf("Failed: %s\n", result.Message)
	}
	return 0
}

func rpcBansRemove(ctx context.Context, client *rpc.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: i2plan rpc bans.remove <node_id>")
		return 1
	}

	nodeID := args[0]

	result, err := client.BansRemove(ctx, nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if result.Success {
		fmt.Printf("Unbanned peer: %s\n", nodeID)
	} else {
		fmt.Printf("Not found: %s\n", result.Message)
	}
	return 0
}

// handleTUI launches the interactive terminal user interface.
func handleTUI(dataDir string) int {
	socketPath, authFile := resolveRPCPaths(dataDir)

	tuiApp, err := createTUIApp(socketPath, authFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the i2plan daemon running?\n")
		return 1
	}
	defer tuiApp.Close()

	return runTUIProgram(tuiApp)
}

// createTUIApp creates and configures a new TUI application instance.
func createTUIApp(socketPath, authFile string) (*tui.Model, error) {
	return tui.New(tui.Config{
		RPCSocketPath:   socketPath,
		RPCAuthFile:     authFile,
		RefreshInterval: 5 * time.Second,
	})
}

// runTUIProgram executes the TUI program and returns the exit code.
func runTUIProgram(tuiApp *tui.Model) int {
	p := tea.NewProgram(tuiApp, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		return 1
	}
	return 0
}

// handleWeb starts the web UI server.
func handleWeb(dataDir string) int {
	socketPath, authFile := resolveRPCPaths(dataDir)
	listenAddr := resolveWebListenAddr()

	server, err := createWebServer(listenAddr, socketPath, authFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the i2plan daemon running?\n")
		return 1
	}

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting web server: %v\n", err)
		return 1
	}

	fmt.Printf("Web UI running at http://%s\n", listenAddr)
	fmt.Println("Press Ctrl+C to stop")

	return waitForShutdownAndStop(server)
}

// resolveRPCPaths determines the socket path and auth file from environment or defaults.
func resolveRPCPaths(dataDir string) (socketPath, authFile string) {
	socketPath = filepath.Join(dataDir, core.DefaultRPCSocket)
	if envSocket := os.Getenv("I2PLAN_RPC_SOCKET"); envSocket != "" {
		socketPath = envSocket
	}

	authFile = filepath.Join(dataDir, "rpc.auth")
	if envAuth := os.Getenv("I2PLAN_RPC_AUTH"); envAuth != "" {
		authFile = envAuth
	}
	return
}

// resolveWebListenAddr determines the web server listen address from environment or defaults.
func resolveWebListenAddr() string {
	listenAddr := "127.0.0.1:8080"
	if envAddr := os.Getenv("I2PLAN_WEB_ADDR"); envAddr != "" {
		listenAddr = envAddr
	}
	return listenAddr
}

// createWebServer creates and configures a new web server instance.
func createWebServer(listenAddr, socketPath, authFile string) (*web.Server, error) {
	return web.New(web.Config{
		ListenAddr:    listenAddr,
		RPCSocketPath: socketPath,
		RPCAuthFile:   authFile,
	})
}

// waitForShutdownAndStop waits for a shutdown signal and gracefully stops the server.
func waitForShutdownAndStop(server *web.Server) int {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping web server: %v\n", err)
		return 1
	}

	return 0
}
