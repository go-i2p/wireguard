package validation

import "time"

// ---- RPC-specific validators ----
// These functions validate RPC handler input parameters and return user-safe error messages.

// ValidatePeersConnectParams validates parameters for peers.connect RPC method.
func ValidatePeersConnectParams(inviteCode string) error {
	return InviteCode("invite_code", inviteCode)
}

// ValidateInviteCreateParams validates parameters for invite.create RPC method.
func ValidateInviteCreateParams(expiry string, maxUses int) (time.Duration, error) {
	// Validate and parse expiry duration
	d, err := DurationRange("expiry", expiry, MinDuration, MaxDuration)
	if err != nil {
		return 0, err
	}

	// Validate max_uses
	if err := MaxUses("max_uses", maxUses); err != nil {
		return 0, err
	}

	return d, nil
}

// ValidateInviteAcceptParams validates parameters for invite.accept RPC method.
func ValidateInviteAcceptParams(inviteCode string) error {
	return InviteCode("invite_code", inviteCode)
}

// ValidateConfigGetParams validates parameters for config.get RPC method.
func ValidateConfigGetParams(key string) error {
	return ConfigKey("key", key)
}

// ValidateConfigSetParams validates parameters for config.set RPC method.
func ValidateConfigSetParams(key string, value any) error {
	if err := Required("key", key); err != nil {
		return err
	}
	return ConfigKey("key", key)
}

// ValidateBanAddParams validates parameters for bans.add RPC method.
func ValidateBanAddParams(nodeID, reason, description, duration string) (time.Duration, error) {
	// Validate node_id (required)
	if err := NodeID("node_id", nodeID); err != nil {
		return 0, err
	}

	// Validate reason (optional)
	if err := Reason("reason", reason); err != nil {
		return 0, err
	}

	// Validate description (optional)
	if err := Description("description", description); err != nil {
		return 0, err
	}

	// Validate and parse duration
	d, err := DurationRange("duration", duration, 0, MaxDuration)
	if err != nil {
		return 0, err
	}

	return d, nil
}

// ValidateBanRemoveParams validates parameters for bans.remove RPC method.
func ValidateBanRemoveParams(nodeID string) error {
	return NodeID("node_id", nodeID)
}
