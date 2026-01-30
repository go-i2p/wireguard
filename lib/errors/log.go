package errors

import (
	"github.com/go-i2p/logger"
)

func init() {
	// Initialize logger for the errors package
	log = logger.GetGoI2PLogger()
}
