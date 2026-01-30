package validation

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRequired(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{"valid string", "name", "test", false},
		{"empty string", "name", "", true},
		{"whitespace only", "name", "   ", true},
		{"tab only", "name", "\t", true},
		{"newline only", "name", "\n", true},
		{"valid with spaces", "name", " test ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Required(tt.field, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Required() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrRequired) {
				t.Errorf("Required() error should wrap ErrRequired")
			}
		})
	}
}

func TestMaxLength(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		max     int
		wantErr bool
	}{
		{"under max", "name", "test", 10, false},
		{"at max", "name", "test", 4, false},
		{"over max", "name", "testing", 4, true},
		{"empty string", "name", "", 10, false},
		{"unicode chars", "name", "日本語", 5, false},
		{"unicode over", "name", "日本語テスト", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MaxLength(tt.field, tt.value, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxLength() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrTooLong) {
				t.Errorf("MaxLength() error should wrap ErrTooLong")
			}
		})
	}
}

func TestMinLength(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		min     int
		wantErr bool
	}{
		{"over min", "name", "testing", 4, false},
		{"at min", "name", "test", 4, false},
		{"under min", "name", "hi", 4, true},
		{"empty string", "name", "", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MinLength(tt.field, tt.value, tt.min)
			if (err != nil) != tt.wantErr {
				t.Errorf("MinLength() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrTooShort) {
				t.Errorf("MinLength() error should wrap ErrTooShort")
			}
		})
	}
}

func TestLengthBetween(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		min     int
		max     int
		wantErr bool
	}{
		{"within range", "test", 2, 10, false},
		{"at min", "ab", 2, 10, false},
		{"at max", "1234567890", 2, 10, false},
		{"under min", "a", 2, 10, true},
		{"over max", "12345678901", 2, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := LengthBetween("field", tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("LengthBetween() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIntRange(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{"within range", 5, 1, 10, false},
		{"at min", 1, 1, 10, false},
		{"at max", 10, 1, 10, false},
		{"below min", 0, 1, 10, true},
		{"above max", 11, 1, 10, true},
		{"negative", -5, 0, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IntRange("field", tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("IntRange() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrOutOfRange) {
				t.Errorf("IntRange() error should wrap ErrOutOfRange")
			}
		})
	}
}

func TestPositive(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"positive", 1, false},
		{"large positive", 1000, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Positive("field", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Positive() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNonNegative(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"positive", 1, false},
		{"zero", 0, false},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NonNegative("field", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("NonNegative() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantDur  time.Duration
		wantErr  bool
		errCheck func(error) bool
	}{
		{"valid hours", "24h", 24 * time.Hour, false, nil},
		{"valid minutes", "30m", 30 * time.Minute, false, nil},
		{"valid seconds", "60s", 60 * time.Second, false, nil},
		{"valid complex", "1h30m", 90 * time.Minute, false, nil},
		{"empty string", "", 0, false, nil},
		{"invalid format", "invalid", 0, true, func(e error) bool { return errors.Is(e, ErrInvalidDuration) }},
		{"negative", "-1h", 0, true, func(e error) bool { return errors.Is(e, ErrOutOfRange) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Duration("field", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Duration() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && d != tt.wantDur {
				t.Errorf("Duration() = %v, want %v", d, tt.wantDur)
			}
			if err != nil && tt.errCheck != nil && !tt.errCheck(err) {
				t.Errorf("Duration() error type mismatch: %v", err)
			}
		})
	}
}

func TestDurationRange(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		min     time.Duration
		max     time.Duration
		wantErr bool
	}{
		{"within range", "1h", time.Minute, 24 * time.Hour, false},
		{"at min", "1m", time.Minute, 24 * time.Hour, false},
		{"at max", "24h", time.Minute, 24 * time.Hour, false},
		{"below min", "30s", time.Minute, 24 * time.Hour, true},
		{"above max", "48h", time.Minute, 24 * time.Hour, true},
		{"empty is valid", "", time.Minute, 24 * time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DurationRange("field", tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("DurationRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeID(t *testing.T) {
	validNodeID := strings.Repeat("a", 64)

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid lowercase", validNodeID, false},
		{"valid uppercase", strings.ToUpper(validNodeID), false},
		{"valid mixed", "aAbBcCdD" + strings.Repeat("0", 56), false},
		{"empty", "", true},
		{"too short", strings.Repeat("a", 63), true},
		{"too long", strings.Repeat("a", 65), true},
		{"invalid chars", strings.Repeat("g", 64), true},
		{"with spaces", "a a" + strings.Repeat("0", 61), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NodeID("node_id", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigKey(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid simple", "name", false},
		{"valid with dot", "mesh.max_peers", false},
		{"valid with underscore", "node_name", false},
		{"valid with number", "node1", false},
		{"empty is valid", "", false},
		{"starts with number", "1node", true},
		{"starts with underscore", "_node", true},
		{"starts with dot", ".node", true},
		{"contains space", "node name", true},
		{"contains hyphen", "node-name", true},
		{"too long", strings.Repeat("a", MaxConfigKeyLength+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfigKey("key", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInviteCode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "i2plan://base64data", false},
		{"empty", "", true},
		{"wrong prefix", "http://test", true},
		{"no prefix", "base64data", true},
		{"too long", "i2plan://" + strings.Repeat("a", MaxInviteCodeLength), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InviteCode("invite_code", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("InviteCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReason(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "spam", false},
		{"empty is valid", "", false},
		{"too long", strings.Repeat("a", MaxReasonLength+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Reason("reason", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reason() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDescription(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "A description", false},
		{"empty is valid", "", false},
		{"too long", strings.Repeat("a", MaxDescriptionLength+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Description("description", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Description() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "my-node", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", MaxNodeNameLength+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NodeName("name", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCIDR(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid ipv4", "10.0.0.0/8", false},
		{"valid ipv4 /16", "192.168.0.0/16", false},
		{"valid ipv4 /24", "10.42.0.0/24", false},
		{"valid ipv6", "fd00::/8", false},
		{"empty", "", true},
		{"no mask", "10.0.0.0", true},
		{"invalid ip", "999.999.999.999/8", true},
		{"invalid mask", "10.0.0.0/33", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CIDR("subnet", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("CIDR() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHostPort(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid localhost", "127.0.0.1:8080", false},
		{"valid hostname", "localhost:7656", false},
		{"valid ipv6", "[::1]:8080", false},
		{"empty", "", true},
		{"no port", "127.0.0.1", true},
		{"no host", ":8080", false}, // This is actually valid in Go
		{"invalid format", "not-a-hostport", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HostPort("address", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("HostPort() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaxUses(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"unlimited (-1)", -1, false},
		{"default (0)", 0, false},
		{"single use", 1, false},
		{"multiple uses", 10, false},
		{"invalid negative", -2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MaxUses("max_uses", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxUses() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPort(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"valid low", 1, false},
		{"valid high", 65535, false},
		{"common port", 8080, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"too high", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Port("port", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Port() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTunnelLength(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{"zero", 0, false},
		{"one", 1, false},
		{"seven", 7, false},
		{"negative", -1, true},
		{"eight", 8, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := TunnelLength("tunnel_length", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("TunnelLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAll(t *testing.T) {
	t.Run("all pass", func(t *testing.T) {
		err := All(
			func() error { return nil },
			func() error { return nil },
		)
		if err != nil {
			t.Errorf("All() = %v, want nil", err)
		}
	})

	t.Run("first fails", func(t *testing.T) {
		expectedErr := errors.New("first error")
		err := All(
			func() error { return expectedErr },
			func() error { return nil },
		)
		if err != expectedErr {
			t.Errorf("All() = %v, want %v", err, expectedErr)
		}
	})

	t.Run("second fails", func(t *testing.T) {
		expectedErr := errors.New("second error")
		err := All(
			func() error { return nil },
			func() error { return expectedErr },
		)
		if err != expectedErr {
			t.Errorf("All() = %v, want %v", err, expectedErr)
		}
	})
}

func TestErrors(t *testing.T) {
	t.Run("empty collection", func(t *testing.T) {
		var errs Errors
		if errs.HasErrors() {
			t.Error("empty Errors should not HasErrors")
		}
		if errs.First() != nil {
			t.Error("empty Errors.First() should be nil")
		}
		if errs.Error() != "" {
			t.Error("empty Errors.Error() should be empty string")
		}
	})

	t.Run("add nil is ignored", func(t *testing.T) {
		var errs Errors
		errs.Add(nil)
		if errs.HasErrors() {
			t.Error("adding nil should not create error")
		}
	})

	t.Run("single error", func(t *testing.T) {
		var errs Errors
		e := errors.New("test error")
		errs.Add(e)

		if !errs.HasErrors() {
			t.Error("should HasErrors")
		}
		if errs.First() != e {
			t.Error("First() should return the error")
		}
		if errs.Error() != "test error" {
			t.Errorf("Error() = %q, want %q", errs.Error(), "test error")
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		var errs Errors
		errs.Add(errors.New("first"))
		errs.Add(errors.New("second"))

		if len(errs) != 2 {
			t.Errorf("len(errs) = %d, want 2", len(errs))
		}
		if !strings.Contains(errs.Error(), "first") || !strings.Contains(errs.Error(), "second") {
			t.Errorf("Error() should contain both errors: %s", errs.Error())
		}
	})
}

func TestResult(t *testing.T) {
	t.Run("with field", func(t *testing.T) {
		r := NewResult("name", "is required", ErrRequired)
		if r.Error() != "name: is required" {
			t.Errorf("Error() = %q, want %q", r.Error(), "name: is required")
		}
		if !errors.Is(r, ErrRequired) {
			t.Error("should wrap ErrRequired")
		}
	})

	t.Run("without field", func(t *testing.T) {
		r := NewResult("", "general error", ErrInvalidFormat)
		if r.Error() != "general error" {
			t.Errorf("Error() = %q, want %q", r.Error(), "general error")
		}
	})
}
