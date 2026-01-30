// Package embedded provides an embeddable i2plan VPN API for third-party applications.
//
// This package wraps the core i2plan mesh VPN functionality in a simple,
// developer-friendly API suitable for embedding in desktop applications,
// mobile apps, daemons, or any Go program that needs VPN capabilities.
//
// # Quick Start
//
// Basic usage requires just a few lines of code:
//
//	vpn, err := embedded.New(embedded.Config{
//	    NodeName: "my-app-vpn",
//	    DataDir:  "/path/to/data",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer vpn.Close()
//
//	if err := vpn.Start(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
//	// VPN is now running
//	fmt.Println("Tunnel IP:", vpn.TunnelIP())
//	fmt.Println("I2P Address:", vpn.I2PAddress())
//
// # Configuration
//
// The [Config] struct provides all configuration options with sensible defaults.
// Zero values are replaced with defaults, so minimal configuration is needed:
//
//	// Minimal config - uses all defaults
//	vpn, _ := embedded.New(embedded.Config{})
//
//	// Custom config
//	vpn, _ := embedded.New(embedded.Config{
//	    NodeName:     "custom-node",
//	    DataDir:      "/var/lib/my-vpn",
//	    SAMAddress:   "127.0.0.1:7656",
//	    TunnelSubnet: "10.42.0.0/16",
//	    MaxPeers:     100,
//	    EnableRPC:    true,
//	    Logger:       slog.Default(),
//	})
//
// Alternatively, use functional options for a fluent API:
//
//	vpn, _ := embedded.NewWithOptions(
//	    embedded.WithNodeName("my-node"),
//	    embedded.WithDataDir("/path/to/data"),
//	    embedded.WithLogger(logger),
//	)
//
// # Lifecycle Management
//
// The VPN follows a simple lifecycle: Initial → Starting → Running → Stopping → Stopped.
//
//   - Call [VPN.Start] to begin the startup sequence
//   - Call [VPN.Stop] or [VPN.Close] for graceful shutdown
//   - Use [VPN.State] or [VPN.Status] to check current state
//   - Use [VPN.Done] channel to wait for shutdown
//
// # Events
//
// Subscribe to VPN events for real-time status updates:
//
//	go func() {
//	    for event := range vpn.Events() {
//	        switch event.Type {
//	        case embedded.EventPeerConnected:
//	            fmt.Printf("Peer connected: %s\n", event.Peer.NodeID)
//	        case embedded.EventError:
//	            fmt.Printf("Error: %v\n", event.Error)
//	        }
//	    }
//	}()
//
// Events are delivered on a buffered channel. If the channel fills up
// (consumer is slow), newer events are dropped. Use [VPN.DroppedEventCount]
// to check if any events have been dropped.
//
// # Peer Management
//
// List connected peers and their status:
//
//	for _, peer := range vpn.Peers() {
//	    fmt.Printf("%s: %s (%s)\n", peer.NodeID, peer.TunnelIP, peer.State)
//	}
//
// # Invites
//
// Create invite codes to allow others to join your network:
//
//	code, err := vpn.CreateInvite(24*time.Hour, 1) // expires in 24h, single use
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Share this invite code:", code)
//
// Accept an invite to join another network:
//
//	err := vpn.AcceptInvite(ctx, inviteCode)
//
// # Thread Safety
//
// All methods on [VPN] are safe for concurrent use.
//
// # Integration with CLI
//
// The i2plan CLI tool uses this package internally. If you're building
// a custom interface, you can use [VPN.Node] to access lower-level
// functionality when needed.
package embedded
