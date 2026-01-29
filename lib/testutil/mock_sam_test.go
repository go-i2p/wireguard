package testutil

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestMockSAM(t *testing.T) {
	sam, err := NewMockSAM()
	if err != nil {
		t.Fatalf("failed to create mock SAM: %v", err)
	}
	defer sam.Close()

	if sam.Addr() == "" {
		t.Error("expected non-empty address")
	}

	// Test that we can connect to the mock SAM
	conn, err := net.DialTimeout("tcp", sam.Addr(), time.Second)
	if err != nil {
		t.Fatalf("failed to connect to mock SAM: %v", err)
	}
	conn.Close()
}

func TestMockTransport(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		transport := NewMockTransport("node1", "dest1.b32.i2p")

		if transport.NodeID() != "node1" {
			t.Errorf("NodeID = %q, want %q", transport.NodeID(), "node1")
		}
		if transport.Dest() != "dest1.b32.i2p" {
			t.Errorf("Dest = %q, want %q", transport.Dest(), "dest1.b32.i2p")
		}
	})

	t.Run("add and remove peers", func(t *testing.T) {
		transport := NewMockTransport("node1", "dest1.b32.i2p")

		transport.AddPeer("node2", "dest2.b32.i2p")
		peers := transport.GetPeers()
		if len(peers) != 1 {
			t.Errorf("expected 1 peer, got %d", len(peers))
		}

		transport.RemovePeer("node2")
		peers = transport.GetPeers()
		if len(peers) != 0 {
			t.Errorf("expected 0 peers, got %d", len(peers))
		}
	})

	t.Run("send to peer", func(t *testing.T) {
		transport := NewMockTransport("node1", "dest1.b32.i2p")
		transport.AddPeer("node2", "dest2.b32.i2p")

		err := transport.SendTo("node2", []byte("hello"))
		if err != nil {
			t.Fatalf("SendTo failed: %v", err)
		}

		// Receive the message
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		msg := transport.ReceiveMessage(ctx)
		if msg == nil {
			t.Fatal("expected to receive a message")
		}
		if msg.From != "node1" {
			t.Errorf("From = %q, want %q", msg.From, "node1")
		}
		if msg.To != "node2" {
			t.Errorf("To = %q, want %q", msg.To, "node2")
		}
		if string(msg.Payload) != "hello" {
			t.Errorf("Payload = %q, want %q", string(msg.Payload), "hello")
		}
	})

	t.Run("send to unknown peer", func(t *testing.T) {
		transport := NewMockTransport("node1", "dest1.b32.i2p")

		err := transport.SendTo("unknown", []byte("hello"))
		if err == nil {
			t.Error("expected error when sending to unknown peer")
		}
	})

	t.Run("broadcast", func(t *testing.T) {
		transport := NewMockTransport("node1", "dest1.b32.i2p")
		transport.AddPeer("node2", "dest2.b32.i2p")
		transport.AddPeer("node3", "dest3.b32.i2p")

		err := transport.Broadcast([]byte("broadcast message"))
		if err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}

		// Should receive 2 messages
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		msg1 := transport.ReceiveMessage(ctx)
		if msg1 == nil {
			t.Fatal("expected to receive first message")
		}

		msg2 := transport.ReceiveMessage(ctx)
		if msg2 == nil {
			t.Fatal("expected to receive second message")
		}
	})
}

func TestMockMeshNetwork(t *testing.T) {
	t.Run("add nodes", func(t *testing.T) {
		network := NewMockMeshNetwork()

		network.AddNode("node1", "dest1.b32.i2p")
		network.AddNode("node2", "dest2.b32.i2p")
		network.AddNode("node3", "dest3.b32.i2p")

		if network.NodeCount() != 3 {
			t.Errorf("NodeCount = %d, want 3", network.NodeCount())
		}
	})

	t.Run("connect nodes", func(t *testing.T) {
		network := NewMockMeshNetwork()

		network.AddNode("node1", "dest1.b32.i2p")
		network.AddNode("node2", "dest2.b32.i2p")

		err := network.ConnectNodes("node1", "node2")
		if err != nil {
			t.Fatalf("ConnectNodes failed: %v", err)
		}

		t1 := network.GetTransport("node1")
		t2 := network.GetTransport("node2")

		// node1 should have node2 as peer
		peers1 := t1.GetPeers()
		if len(peers1) != 1 {
			t.Errorf("node1 should have 1 peer, got %d", len(peers1))
		}

		// node2 should have node1 as peer
		peers2 := t2.GetPeers()
		if len(peers2) != 1 {
			t.Errorf("node2 should have 1 peer, got %d", len(peers2))
		}
	})

	t.Run("disconnect nodes", func(t *testing.T) {
		network := NewMockMeshNetwork()

		network.AddNode("node1", "dest1.b32.i2p")
		network.AddNode("node2", "dest2.b32.i2p")
		network.ConnectNodes("node1", "node2")
		network.DisconnectNodes("node1", "node2")

		t1 := network.GetTransport("node1")
		peers := t1.GetPeers()
		if len(peers) != 0 {
			t.Errorf("node1 should have 0 peers, got %d", len(peers))
		}
	})

	t.Run("connect unknown nodes", func(t *testing.T) {
		network := NewMockMeshNetwork()

		err := network.ConnectNodes("node1", "node2")
		if err == nil {
			t.Error("expected error when connecting unknown nodes")
		}
	})

	t.Run("get unknown transport", func(t *testing.T) {
		network := NewMockMeshNetwork()

		tr := network.GetTransport("unknown")
		if tr != nil {
			t.Error("expected nil for unknown transport")
		}
	})
}

func TestMockMeshNetwork_MessagePassing(t *testing.T) {
	network := NewMockMeshNetwork()

	network.AddNode("node1", "dest1.b32.i2p")
	network.AddNode("node2", "dest2.b32.i2p")
	network.AddNode("node3", "dest3.b32.i2p")

	// Create a star topology: node1 <-> node2 <-> node3
	network.ConnectNodes("node1", "node2")
	network.ConnectNodes("node2", "node3")

	t1 := network.GetTransport("node1")
	t2 := network.GetTransport("node2")

	// Send message from node1 to node2
	err := t1.SendTo("node2", []byte("hello from node1"))
	if err != nil {
		t.Fatalf("SendTo failed: %v", err)
	}

	// node1 should have the message in its queue (our mock is simple)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msg := t1.ReceiveMessage(ctx)
	if msg == nil {
		t.Fatal("expected to receive a message")
	}
	if string(msg.Payload) != "hello from node1" {
		t.Errorf("unexpected payload: %q", string(msg.Payload))
	}

	// node2 can send to both node1 and node3
	peers := t2.GetPeers()
	if len(peers) != 2 {
		t.Errorf("node2 should have 2 peers, got %d", len(peers))
	}
}
