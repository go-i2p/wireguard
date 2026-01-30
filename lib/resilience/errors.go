package resilience

import apperrors "github.com/go-i2p/wireguard/lib/errors"

// ErrCircuitOpen is returned when a request is rejected because the circuit is open.
// This is an alias to the central error definition in lib/errors.
var ErrCircuitOpen = apperrors.ErrCircuitOpen
