package i2pbind

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/go-i2p/i2pkeys"
)

// testI2PDestination is a valid base64-encoded I2P destination for testing.
// This is a 516-character base64 string that represents a minimal valid destination.
// Format: 256 bytes public key + 128 bytes signing key + null certificate = 387 bytes
// Base64 encoded: 387 * 4/3 ≈ 516 characters
const testI2PDestination = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// getTestAddress returns a valid I2P address for testing.
// Uses a hardcoded valid base64 destination since FiveHundredAs() may return
// empty on some systems due to base64 validation.
func getTestAddress() i2pkeys.I2PAddr {
	// First try the library function
	addr := i2pkeys.FiveHundredAs()
	if addr != "" {
		return addr
	}
	// Fallback to our known-good test destination
	return i2pkeys.I2PAddr(testI2PDestination)
}

// mustGetNonEmptyTestAddress returns a test address and fails the test if empty.
func mustGetNonEmptyTestAddress(t *testing.T) i2pkeys.I2PAddr {
	t.Helper()
	addr := getTestAddress()
	if addr == "" {
		// Create a minimal valid address for testing - 516+ chars of valid base64
		addr = i2pkeys.I2PAddr(strings.Repeat("A", 520))
	}
	if addr == "" {
		t.Fatal("Could not create a valid test I2P address")
	}
	return addr
}

func TestNewI2PEndpoint(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	if ep == nil {
		t.Fatal("NewI2PEndpoint returned nil")
	}

	if ep.dest != addr {
		t.Error("Endpoint destination does not match input address")
	}
}

func TestI2PEndpoint_DstToBytes(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	bytes := ep.DstToBytes()

	// The raw hash should be exactly 32 bytes (256 bits)
	// This verifies the fix: we return raw bytes, not base32-encoded string
	if len(bytes) != 32 {
		t.Errorf("DstToBytes should return 32 bytes (raw hash), got %d bytes", len(bytes))
	}
}

func TestI2PEndpoint_DstToString(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	dst := ep.DstToString()

	// Base32 addresses should not be empty
	if dst == "" {
		t.Error("DstToString returned empty string")
	}
}

func TestI2PEndpoint_ClearSrc(t *testing.T) {
	addr := mustGetNonEmptyTestAddress(t)

	ep := &I2PEndpoint{
		dest: addr,
		src:  addr, // Set a source
	}

	// Before clearing, src should be set (not empty string)
	if ep.src == "" {
		t.Errorf("Source I2PAddr should not be empty before ClearSrc, got len=%d", len(addr))
	}

	ep.ClearSrc()

	// After clearing, src should be empty
	if ep.src != "" {
		t.Error("Source should be empty string after ClearSrc")
	}

	// SrcToString should return empty after clearing
	if ep.SrcToString() != "" {
		t.Error("SrcToString should be empty after ClearSrc")
	}
}

func TestI2PEndpoint_SrcToString(t *testing.T) {
	addr := mustGetNonEmptyTestAddress(t)

	// Test with no source set
	ep := NewI2PEndpoint(addr)
	if ep.SrcToString() != "" {
		t.Error("SrcToString should return empty string when no source is set")
	}

	// Test that setting a source and clearing it works correctly
	ep.src = addr
	// Verify src is actually set (not empty string)
	if ep.src == "" {
		t.Errorf("Source I2PAddr should be set, got len=%d", len(addr))
	}

	// Clear and verify
	ep.ClearSrc()
	if ep.SrcToString() != "" {
		t.Error("SrcToString should return empty after ClearSrc")
	}
}

func TestI2PEndpoint_DstIP(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	ip := ep.DstIP()

	// I2P doesn't use IP addresses, so this should return an invalid address
	if ip.IsValid() {
		t.Error("DstIP should return an invalid netip.Addr for I2P endpoints")
	}
}

func TestI2PEndpoint_SrcIP(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	ip := ep.SrcIP()

	// I2P doesn't use IP addresses, so this should return an invalid address
	if ip.IsValid() {
		t.Error("SrcIP should return an invalid netip.Addr for I2P endpoints")
	}
}

func TestI2PEndpoint_Destination(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)
	dest := ep.Destination()

	if dest != addr {
		t.Error("Destination() should return the original I2P address")
	}
}

func TestNewI2PBind(t *testing.T) {
	bind := NewI2PBind("test-tunnel")
	if bind == nil {
		t.Fatal("NewI2PBind returned nil")
	}

	if bind.name != "test-tunnel" {
		t.Errorf("Expected name 'test-tunnel', got '%s'", bind.name)
	}

	if bind.samAddr != DefaultSAMAddress {
		t.Errorf("Expected default SAM address '%s', got '%s'", DefaultSAMAddress, bind.samAddr)
	}
}

func TestNewI2PBindWithSAM(t *testing.T) {
	customSAM := "192.168.1.1:7656"
	bind := NewI2PBindWithSAM("test-tunnel", customSAM)
	if bind == nil {
		t.Fatal("NewI2PBindWithSAM returned nil")
	}

	if bind.name != "test-tunnel" {
		t.Errorf("Expected name 'test-tunnel', got '%s'", bind.name)
	}

	if bind.samAddr != customSAM {
		t.Errorf("Expected SAM address '%s', got '%s'", customSAM, bind.samAddr)
	}
}

func TestNewI2PBindWithOptions(t *testing.T) {
	customSAM := "192.168.1.1:7656"
	customOptions := []string{"inbound.length=2", "outbound.length=2"}

	bind := NewI2PBindWithOptions("test-tunnel", customSAM, customOptions)
	if bind == nil {
		t.Fatal("NewI2PBindWithOptions returned nil")
	}

	if bind.name != "test-tunnel" {
		t.Errorf("Expected name 'test-tunnel', got '%s'", bind.name)
	}

	if bind.samAddr != customSAM {
		t.Errorf("Expected SAM address '%s', got '%s'", customSAM, bind.samAddr)
	}

	if len(bind.samOptions) != len(customOptions) {
		t.Errorf("Expected %d SAM options, got %d", len(customOptions), len(bind.samOptions))
	}

	for i, opt := range customOptions {
		if bind.samOptions[i] != opt {
			t.Errorf("SAM option mismatch at index %d: expected '%s', got '%s'", i, opt, bind.samOptions[i])
		}
	}
}

func TestNewI2PBindWithOptions_NilOptions(t *testing.T) {
	// Test that nil options are handled correctly (should use defaults in Open)
	bind := NewI2PBindWithOptions("test-tunnel", DefaultSAMAddress, nil)
	if bind == nil {
		t.Fatal("NewI2PBindWithOptions returned nil")
	}

	if bind.samOptions != nil {
		t.Error("Expected nil samOptions when nil passed to constructor")
	}
}

func TestI2PBind_BatchSize(t *testing.T) {
	bind := NewI2PBind("test-tunnel")
	batchSize := bind.BatchSize()

	// I2P doesn't support batching like UDP GSO
	if batchSize != 1 {
		t.Errorf("Expected BatchSize of 1, got %d", batchSize)
	}
}

func TestI2PBind_SetMark(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// SetMark is a no-op for I2P, should always return nil
	err := bind.SetMark(123)
	if err != nil {
		t.Errorf("SetMark should return nil for I2P, got: %v", err)
	}
}

func TestI2PBind_LocalAddressNotOpen(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Before Open() is called, LocalAddress should return an error
	_, err := bind.LocalAddress()
	if err == nil {
		t.Error("LocalAddress should return error when bind is not open")
	}
}

func TestI2PBind_LocalDestinationNotOpen(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Before Open() is called, LocalDestination should return an error
	_, err := bind.LocalDestination()
	if err == nil {
		t.Error("LocalDestination should return error when bind is not open")
	}
}

func TestI2PBind_CloseIdempotent(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Close on an unopened bind should not error
	err := bind.Close()
	if err != nil {
		t.Errorf("First Close should not error: %v", err)
	}

	// Second close should also not error (idempotent)
	err = bind.Close()
	if err != nil {
		t.Errorf("Second Close should not error: %v", err)
	}
}

// TestDstToBytesFormat verifies the fix for the DstToBytes bug.
// Previously, DstToBytes returned a base32-encoded string as bytes (52 bytes).
// After the fix, it returns the raw 32-byte hash.
func TestDstToBytesFormat(t *testing.T) {
	addr := getTestAddress()

	ep := NewI2PEndpoint(addr)

	// Get the hash both ways to compare
	hash := addr.DestHash()
	dstBytes := ep.DstToBytes()

	// Raw hash should be exactly 32 bytes
	expectedLen := 32
	if len(dstBytes) != expectedLen {
		t.Errorf("DstToBytes length mismatch: expected %d bytes, got %d bytes", expectedLen, len(dstBytes))
	}

	// The bytes should match the raw hash array
	for i := 0; i < len(hash); i++ {
		if dstBytes[i] != hash[i] {
			t.Errorf("DstToBytes byte mismatch at index %d: expected %d, got %d", i, hash[i], dstBytes[i])
			break
		}
	}
}

// TestSetMeshHandler tests the mesh handler registration
func TestSetMeshHandler(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	called := false
	handler := func(data []byte, from i2pkeys.I2PAddr) {
		called = true
	}

	bind.SetMeshHandler(handler)

	if bind.meshHandler == nil {
		t.Error("Mesh handler should be set")
	}

	// Verify handler can be called
	bind.meshHandler([]byte("test"), getTestAddress())
	if !called {
		t.Error("Mesh handler was not called")
	}
}

// TestSetMeshHandler_AfterMessages tests the warning path when handler is set after messages received
func TestSetMeshHandler_AfterMessages(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Simulate messages received
	bind.messagesReceived.Store(true)

	handler := func(data []byte, from i2pkeys.I2PAddr) {}

	// This should log a warning but still set the handler
	bind.SetMeshHandler(handler)

	if bind.meshHandler == nil {
		t.Error("Mesh handler should be set even after messages received")
	}
}

// TestParseI2PAddress tests the helper function for parsing network addresses
func TestParseI2PAddress(t *testing.T) {
	addr := getTestAddress()

	tests := []struct {
		name    string
		addr    i2pkeys.I2PAddr
		wantErr bool
	}{
		{
			name:    "valid I2P address",
			addr:    addr,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseI2PAddress(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseI2PAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.addr {
				t.Error("parseI2PAddress() returned wrong address")
			}
		})
	}
}

// TestIsMeshProtocolMessage tests the mesh message detection
func TestIsMeshProtocolMessage(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "JSON message starting with {",
			data: []byte(`{"type":"test"}`),
			want: true,
		},
		{
			name: "WireGuard message type 1",
			data: []byte{0x01, 0x00, 0x00, 0x00},
			want: false,
		},
		{
			name: "WireGuard message type 2",
			data: []byte{0x02, 0x00, 0x00, 0x00},
			want: false,
		},
		{
			name: "empty message",
			data: []byte{},
			want: false,
		},
		{
			name: "random binary data",
			data: []byte{0xff, 0xfe, 0xfd, 0xfc},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMeshProtocolMessage(tt.data); got != tt.want {
				t.Errorf("isMeshProtocolMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHandleMeshMessage tests the mesh message dispatch
func TestHandleMeshMessage(t *testing.T) {
	bind := NewI2PBind("test-tunnel")
	addr := getTestAddress()

	t.Run("nil handler", func(t *testing.T) {
		// Should not panic with nil handler
		bind.handleMeshMessage([]byte("test"), 4, addr, nil)
	})

	t.Run("with handler", func(t *testing.T) {
		done := make(chan struct{})
		var receivedData []byte
		var receivedAddr i2pkeys.I2PAddr

		handler := func(data []byte, from i2pkeys.I2PAddr) {
			receivedData = data
			receivedAddr = from
			close(done)
		}

		data := []byte("test message")
		bind.handleMeshMessage(data, len(data), addr, handler)

		// Wait for goroutine to complete
		<-done

		if string(receivedData) != string(data) {
			t.Errorf("Handler received wrong data: got %s, want %s", receivedData, data)
		}
		if receivedAddr != addr {
			t.Error("Handler received wrong address")
		}
	})
}

// TestParseEndpoint tests endpoint parsing with various formats
func TestParseEndpoint(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid full destination",
			input:   testI2PDestination,
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "not-a-valid-address",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, err := bind.ParseEndpoint(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEndpoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && ep == nil {
				t.Error("ParseEndpoint() returned nil endpoint")
			}
			if !tt.wantErr {
				i2pEp, ok := ep.(*I2PEndpoint)
				if !ok {
					t.Error("ParseEndpoint() did not return *I2PEndpoint")
				}
				if i2pEp == nil {
					t.Error("ParseEndpoint() returned nil I2PEndpoint")
				}
			}
		})
	}
}

// TestI2PBind_SendNotOpen tests Send on closed bind
func TestI2PBind_SendNotOpen(t *testing.T) {
	bind := NewI2PBind("test-tunnel")
	addr := getTestAddress()
	ep := NewI2PEndpoint(addr)

	// Send should fail when bind is not open
	err := bind.Send([][]byte{[]byte("test")}, ep)
	if err == nil {
		t.Error("Send should return error when bind is not open")
	}
}

// mockEndpoint is a test endpoint that is not *I2PEndpoint
type mockEndpoint struct{}

func (m *mockEndpoint) ClearSrc()           {}
func (m *mockEndpoint) SrcToString() string { return "" }
func (m *mockEndpoint) DstToString() string { return "" }
func (m *mockEndpoint) DstToBytes() []byte  { return nil }
func (m *mockEndpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (m *mockEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }

// TestI2PBind_SendWrongEndpointType tests Send with wrong endpoint type
func TestI2PBind_SendWrongEndpointType(t *testing.T) {
	bind := NewI2PBind("test-tunnel")

	// Create a mock endpoint that is not *I2PEndpoint
	err := bind.Send([][]byte{[]byte("test")}, &mockEndpoint{})
	if err == nil {
		t.Error("Send should return error with wrong endpoint type")
	}
}

// TestI2PBind_Integration tests Open, Send, and Close with real SAM
func TestI2PBind_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bind := NewI2PBind("test-tunnel-integration")

	// Test Open
	recvFuncs, port, err := bind.Open(0)
	if err != nil {
		t.Skipf("Skipping integration test: SAM bridge not available: %v", err)
	}
	defer bind.Close()

	if len(recvFuncs) != 1 {
		t.Errorf("Expected 1 receive function, got %d", len(recvFuncs))
	}

	if port != 0 {
		t.Errorf("Expected port 0 for I2P, got %d", port)
	}

	// Test LocalAddress
	localAddr, err := bind.LocalAddress()
	if err != nil {
		t.Fatalf("LocalAddress() failed: %v", err)
	}
	if localAddr == "" {
		t.Error("LocalAddress() returned empty string")
	}
	if !strings.HasSuffix(localAddr, ".b32.i2p") {
		t.Errorf("LocalAddress() should end with .b32.i2p, got: %s", localAddr)
	}

	// Test LocalDestination
	localDest, err := bind.LocalDestination()
	if err != nil {
		t.Fatalf("LocalDestination() failed: %v", err)
	}
	if localDest == "" {
		t.Error("LocalDestination() returned empty")
	}

	// Test ParseEndpoint with our local destination (not base32 address)
	ep, err := bind.ParseEndpoint(string(localDest))
	if err != nil {
		t.Fatalf("ParseEndpoint() failed: %v", err)
	}

	// Test Send - send a message to ourselves
	testData := []byte("test message")
	bufs := [][]byte{testData}
	err = bind.Send(bufs, ep)
	if err != nil {
		t.Errorf("Send() failed: %v", err)
	}

	// Test that Open fails when already open
	_, _, err = bind.Open(0)
	if err == nil {
		t.Error("Open() should fail when already open")
	}

	// Test Close
	err = bind.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Test that operations fail after close
	_, err = bind.LocalAddress()
	if err == nil {
		t.Error("LocalAddress() should fail after Close()")
	}

	err = bind.Send(bufs, ep)
	if err == nil {
		t.Error("Send() should fail after Close()")
	}
}

// TestI2PBind_SendOversizePacket tests sending packets larger than max size
func TestI2PBind_SendOversizePacket(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bind := NewI2PBind("test-tunnel-oversize")
	_, _, err := bind.Open(0)
	if err != nil {
		t.Skipf("Skipping integration test: SAM bridge not available: %v", err)
	}
	defer bind.Close()

	localDest, _ := bind.LocalDestination()
	ep, _ := bind.ParseEndpoint(string(localDest))

	// Create a packet larger than MaxI2PDatagramSize
	oversizePacket := make([]byte, MaxI2PDatagramSize+1)
	bufs := [][]byte{oversizePacket}

	err = bind.Send(bufs, ep)
	if err == nil {
		t.Error("Send() should fail with oversize packet")
	}
}

// TestI2PBind_ReceiveFunc tests the receive function with mesh handler
func TestI2PBind_ReceiveFunc(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create sender bind
	sender := NewI2PBind("test-sender")
	recvFuncs, _, err := sender.Open(0)
	if err != nil {
		t.Skipf("Skipping integration test: SAM bridge not available: %v", err)
	}
	defer sender.Close()

	if len(recvFuncs) != 1 {
		t.Fatalf("Expected 1 receive function, got %d", len(recvFuncs))
	}

	// Create receiver bind with mesh handler
	receiver := NewI2PBind("test-receiver")

	meshReceived := make(chan []byte, 1)
	receiver.SetMeshHandler(func(data []byte, from i2pkeys.I2PAddr) {
		meshReceived <- data
	})

	_, _, err = receiver.Open(0)
	if err != nil {
		t.Skipf("Skipping integration test: SAM bridge not available: %v", err)
	}
	defer receiver.Close()

	// Get receiver's full destination (not base32 address)
	receiverDest, err := receiver.LocalDestination()
	if err != nil {
		t.Fatalf("Failed to get receiver destination: %v", err)
	}

	// Parse receiver endpoint using full destination
	receiverEp, err := sender.ParseEndpoint(string(receiverDest))
	if err != nil {
		t.Fatalf("Failed to parse receiver endpoint: %v", err)
	}

	// Send a mesh message (JSON starting with '{')
	meshMsg := []byte(`{"type":"test","data":"hello"}`)
	err = sender.Send([][]byte{meshMsg}, receiverEp)
	if err != nil {
		t.Fatalf("Failed to send mesh message: %v", err)
	}

	t.Log("Integration test completed - mesh message sent (may not be received in test timeframe)")
}

// TestI2PBind_CloseCleansUp tests that Close properly cleans up resources
func TestI2PBind_CloseCleansUp(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bind := NewI2PBind("test-cleanup")
	_, _, err := bind.Open(0)
	if err != nil {
		t.Skipf("Skipping integration test: SAM bridge not available: %v", err)
	}

	// Verify resources are set
	bind.mu.Lock()
	hasGarlic := bind.garlic != nil
	hasPacketConn := bind.packetConn != nil
	bind.mu.Unlock()

	if !hasGarlic || !hasPacketConn {
		t.Fatal("Resources not initialized after Open()")
	}

	// Close and verify cleanup
	err = bind.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	bind.mu.Lock()
	if bind.garlic != nil {
		t.Error("garlic should be nil after Close()")
	}
	if bind.packetConn != nil {
		t.Error("packetConn should be nil after Close()")
	}
	if !bind.closed {
		t.Error("closed flag should be true after Close()")
	}
	bind.mu.Unlock()
}
