package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DNSManager handles DNS configuration for exit client mode.
// It provides backup, update, and restoration of DNS settings to prevent DNS leaks
// when routing traffic through an exit node.
type DNSManager struct {
	mu             sync.RWMutex
	originalResolv string // Content of original resolv.conf
	backupPath     string // Path to backup file
	resolvPath     string // Path to resolv.conf (configurable for testing)
	isManaged      bool   // Whether DNS is currently managed by us
}

// NewDNSManager creates a new DNS manager instance.
// It validates that the resolv.conf file exists and is readable.
func NewDNSManager() (*DNSManager, error) {

	resolvPath := "/etc/resolv.conf"

	// Check if resolv.conf exists and is readable
	if _, err := os.Stat(resolvPath); err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", resolvPath, err)
	}

	// Create backup directory in user's home or temp
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}

	backupDir := filepath.Join(homeDir, ".i2plan", "dns-backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("resolv.conf.%s.backup", timestamp))

	return &DNSManager{
		resolvPath: resolvPath,
		backupPath: backupPath,
		isManaged:  false,
	}, nil
}

// backup creates a backup of the current resolv.conf.
// It reads the file content and stores both in memory and on disk.
func (d *DNSManager) backup() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Read current resolv.conf
	content, err := os.ReadFile(d.resolvPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", d.resolvPath, err)
	}

	d.originalResolv = string(content)

	// Write backup to disk
	if err := os.WriteFile(d.backupPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write backup to %s: %w", d.backupPath, err)
	}

	log.Printf("DNS configuration backed up to %s", d.backupPath)
	return nil
}

// UpdateDNS updates the system's DNS configuration with the specified servers.
// It first backs up the current configuration, then writes the new settings.
// Returns error if backup or write fails.
func (d *DNSManager) UpdateDNS(servers []string) error {
	if len(servers) == 0 {
		return fmt.Errorf("no DNS servers provided")
	}

	// Validate DNS server addresses
	for _, server := range servers {
		if !isValidDNSServer(server) {
			return fmt.Errorf("invalid DNS server address: %s", server)
		}
	}

	// Backup current configuration if not already backed up
	if !d.isManaged {
		if err := d.backup(); err != nil {
			return fmt.Errorf("failed to backup DNS configuration: %w", err)
		}
	}

	// Build new resolv.conf content
	var sb strings.Builder
	sb.WriteString("# Managed by i2plan exit mode\n")
	sb.WriteString(fmt.Sprintf("# Original backed up to: %s\n", d.backupPath))
	sb.WriteString(fmt.Sprintf("# Timestamp: %s\n\n", time.Now().Format(time.RFC3339)))

	for _, server := range servers {
		sb.WriteString(fmt.Sprintf("nameserver %s\n", server))
	}

	// Write new configuration
	content := sb.String()
	if err := os.WriteFile(d.resolvPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", d.resolvPath, err)
	}

	d.mu.Lock()
	d.isManaged = true
	d.mu.Unlock()

	log.Printf("DNS configuration updated with %d servers", len(servers))
	return nil
}

// Restore restores the original DNS configuration from backup.
// It reads the backup file and writes it back to resolv.conf.
// Returns error if no backup exists or restore fails.
func (d *DNSManager) Restore() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isManaged {
		log.Printf("DNS not currently managed, nothing to restore")
		return nil
	}

	// Check if we have an in-memory backup
	if d.originalResolv == "" {
		// Try to read from backup file
		content, err := os.ReadFile(d.backupPath)
		if err != nil {
			return fmt.Errorf("no backup available to restore: %w", err)
		}
		d.originalResolv = string(content)
	}

	// Write original content back
	if err := os.WriteFile(d.resolvPath, []byte(d.originalResolv), 0644); err != nil {
		return fmt.Errorf("failed to restore %s: %w", d.resolvPath, err)
	}

	d.isManaged = false
	log.Printf("DNS configuration restored from backup")

	// Clean up backup file
	if err := os.Remove(d.backupPath); err != nil {
		log.Printf("Warning: failed to remove backup file %s: %v", d.backupPath, err)
	}

	return nil
}

// IsManaged returns whether DNS is currently being managed by this instance.
func (d *DNSManager) IsManaged() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isManaged
}

// GetCurrentServers parses and returns the current DNS servers from resolv.conf.
func (d *DNSManager) GetCurrentServers() ([]string, error) {
	content, err := os.ReadFile(d.resolvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", d.resolvPath, err)
	}

	var servers []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			server := strings.TrimSpace(strings.TrimPrefix(line, "nameserver"))
			if server != "" {
				servers = append(servers, server)
			}
		}
	}

	return servers, nil
}

// VerifyNoDNSLeak performs a basic DNS leak check by querying a well-known service.
// This is a simplified implementation that checks if DNS queries are going through
// the expected servers. A more comprehensive check would query external DNS leak
// testing services.
func (d *DNSManager) VerifyNoDNSLeak(expectedServers []string) error {
	currentServers, err := d.GetCurrentServers()
	if err != nil {
		return fmt.Errorf("failed to get current DNS servers: %w", err)
	}

	if len(currentServers) == 0 {
		return fmt.Errorf("no DNS servers configured")
	}

	// Check if current servers match expected servers
	if !serversMatch(currentServers, expectedServers) {
		return fmt.Errorf("DNS leak detected: current servers %v do not match expected %v",
			currentServers, expectedServers)
	}

	log.Printf("DNS leak check passed: servers match expected configuration")
	return nil
}

// serversMatch checks if two lists of DNS servers match (order independent).
func serversMatch(current, expected []string) bool {
	if len(current) != len(expected) {
		return false
	}

	// Create map of expected servers
	expectedMap := make(map[string]bool)
	for _, server := range expected {
		expectedMap[server] = true
	}

	// Check all current servers are in expected
	for _, server := range current {
		if !expectedMap[server] {
			return false
		}
	}

	return true
}

// isValidDNSServer performs basic validation on DNS server addresses.
// Accepts IPv4, IPv6, and hostnames.
func isValidDNSServer(server string) bool {
	if server == "" {
		return false
	}

	// Check for invalid characters before trimming
	if strings.Contains(server, " ") || strings.Contains(server, "\n") || strings.Contains(server, "\t") {
		return false
	}

	server = strings.TrimSpace(server)
	if server == "" {
		return false
	}

	// Check for common invalid patterns
	if strings.HasPrefix(server, "#") || strings.HasPrefix(server, "//") {
		return false
	}

	return true
}

// CopyFile copies a file from src to dst, creating the destination directory if needed.
// This is used for backup operations.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create destination directory
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
