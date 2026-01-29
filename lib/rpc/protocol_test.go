package rpc

import (
	"encoding/json"
	"testing"
)

func TestProtocolVersion(t *testing.T) {
	if ProtocolVersion != "1.0" {
		t.Errorf("expected protocol version 1.0, got %s", ProtocolVersion)
	}
}

func TestNewError(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		message string
		data    any
	}{
		{"simple error", ErrCodeInternal, "internal error", nil},
		{"error with data", ErrCodeInvalidParams, "invalid params", "missing field"},
		{"method not found", ErrCodeMethodNotFound, "method not found", "unknown.method"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError(tt.code, tt.message, tt.data)
			if err.Code != tt.code {
				t.Errorf("expected code %d, got %d", tt.code, err.Code)
			}
			if err.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, err.Message)
			}
			if err.Data != tt.data {
				t.Errorf("expected data %v, got %v", tt.data, err.Data)
			}
		})
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			"without data",
			NewError(ErrCodeInternal, "internal error", nil),
			"internal error (code -32603)",
		},
		{
			"with data",
			NewError(ErrCodeNotFound, "not found", "resource.id"),
			"not found (code -32003): resource.id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     Request
		wantErr bool
	}{
		{
			"valid request",
			Request{JSONRPC: "2.0", Method: "status"},
			false,
		},
		{
			"missing jsonrpc",
			Request{Method: "status"},
			true,
		},
		{
			"wrong jsonrpc version",
			Request{JSONRPC: "1.0", Method: "status"},
			true,
		},
		{
			"missing method",
			Request{JSONRPC: "2.0"},
			true,
		},
		{
			"with params",
			Request{JSONRPC: "2.0", Method: "config.get", Params: json.RawMessage(`{"key":"test"}`)},
			false,
		},
		{
			"with id",
			Request{JSONRPC: "2.0", Method: "status", ID: json.RawMessage(`1`)},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewSuccessResponse(t *testing.T) {
	id := json.RawMessage(`1`)
	result := map[string]string{"status": "ok"}

	resp := NewSuccessResponse(id, result)

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Error("expected no error")
	}
	if string(resp.ID) != "1" {
		t.Errorf("expected ID 1, got %s", string(resp.ID))
	}
}

func TestNewErrorResponse(t *testing.T) {
	id := json.RawMessage(`"abc"`)
	err := ErrMethodNotFound("unknown")

	resp := NewErrorResponse(id, err)

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.Result != nil {
		t.Error("expected no result")
	}
	if resp.Error == nil {
		t.Error("expected error")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, resp.Error.Code)
	}
	if string(resp.ID) != `"abc"` {
		t.Errorf("expected ID \"abc\", got %s", string(resp.ID))
	}
}

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		code    int
		message string
	}{
		{"method not found", ErrMethodNotFound("test"), ErrCodeMethodNotFound, "method not found"},
		{"invalid params", ErrInvalidParams("missing field"), ErrCodeInvalidParams, "invalid params"},
		{"internal", ErrInternal("crash"), ErrCodeInternal, "internal error"},
		{"auth required", ErrAuthRequired(), ErrCodeAuthRequired, "authentication required"},
		{"permission denied", ErrPermissionDenied("access denied"), ErrCodePermissionDenied, "permission denied"},
		{"not found", ErrNotFound("resource"), ErrCodeNotFound, "not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.code {
				t.Errorf("expected code %d, got %d", tt.code, tt.err.Code)
			}
			if tt.err.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, tt.err.Message)
			}
		})
	}
}

func TestRequestResponseJSON(t *testing.T) {
	// Test Request marshaling/unmarshaling
	t.Run("request roundtrip", func(t *testing.T) {
		req := Request{
			JSONRPC: "2.0",
			Method:  "peers.list",
			Params:  json.RawMessage(`{"filter":"active"}`),
			ID:      json.RawMessage(`42`),
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded Request
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if decoded.JSONRPC != req.JSONRPC {
			t.Errorf("JSONRPC mismatch: %s != %s", decoded.JSONRPC, req.JSONRPC)
		}
		if decoded.Method != req.Method {
			t.Errorf("Method mismatch: %s != %s", decoded.Method, req.Method)
		}
		if string(decoded.Params) != string(req.Params) {
			t.Errorf("Params mismatch: %s != %s", string(decoded.Params), string(req.Params))
		}
		if string(decoded.ID) != string(req.ID) {
			t.Errorf("ID mismatch: %s != %s", string(decoded.ID), string(req.ID))
		}
	})

	// Test Response marshaling/unmarshaling
	t.Run("success response roundtrip", func(t *testing.T) {
		resp := NewSuccessResponse(
			json.RawMessage(`1`),
			StatusResult{NodeName: "test", State: "running"},
		)

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded Response
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if decoded.Error != nil {
			t.Error("expected no error in decoded response")
		}
	})

	t.Run("error response roundtrip", func(t *testing.T) {
		resp := NewErrorResponse(
			json.RawMessage(`"req-1"`),
			ErrMethodNotFound("unknown.method"),
		)

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded Response
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if decoded.Error == nil {
			t.Fatal("expected error in decoded response")
		}
		if decoded.Error.Code != ErrCodeMethodNotFound {
			t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, decoded.Error.Code)
		}
	})
}

func TestStatusResultJSON(t *testing.T) {
	result := StatusResult{
		NodeName:       "my-node",
		NodeID:         "abc123",
		State:          "running",
		TunnelIP:       "10.42.1.1",
		I2PDestination: "abcd...wxyz",
		PeerCount:      5,
		Uptime:         "2h30m",
		Version:        "1.0.0",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded StatusResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.NodeName != result.NodeName {
		t.Errorf("NodeName mismatch")
	}
	if decoded.PeerCount != result.PeerCount {
		t.Errorf("PeerCount mismatch: %d != %d", decoded.PeerCount, result.PeerCount)
	}
}

func TestPeersListResultJSON(t *testing.T) {
	result := PeersListResult{
		Peers: []PeerInfo{
			{NodeID: "peer1", TunnelIP: "10.42.1.2", State: "connected"},
			{NodeID: "peer2", TunnelIP: "10.42.1.3", State: "pending"},
		},
		Total: 2,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PeersListResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Total != 2 {
		t.Errorf("Total mismatch: %d != 2", decoded.Total)
	}
	if len(decoded.Peers) != 2 {
		t.Errorf("Peers length mismatch: %d != 2", len(decoded.Peers))
	}
}

func TestInviteCreateParamsJSON(t *testing.T) {
	params := InviteCreateParams{
		Expiry:  "48h",
		MaxUses: 5,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded InviteCreateParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Expiry != "48h" {
		t.Errorf("Expiry mismatch: %s != 48h", decoded.Expiry)
	}
	if decoded.MaxUses != 5 {
		t.Errorf("MaxUses mismatch: %d != 5", decoded.MaxUses)
	}
}

func TestConfigSetParamsJSON(t *testing.T) {
	params := ConfigSetParams{
		Key:   "mesh.max_peers",
		Value: 100,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ConfigSetParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Key != "mesh.max_peers" {
		t.Errorf("Key mismatch: %s != mesh.max_peers", decoded.Key)
	}
	// Value is any, comes back as float64 from JSON
	if v, ok := decoded.Value.(float64); !ok || v != 100 {
		t.Errorf("Value mismatch: %v != 100", decoded.Value)
	}
}
