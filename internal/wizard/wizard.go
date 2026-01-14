// Package wizard provides an interactive setup wizard for Muti Metroo.
package wizard

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/postalsys/muti-metroo/internal/certutil"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/embed"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/probe"
	"github.com/postalsys/muti-metroo/internal/service"
	"github.com/postalsys/muti-metroo/internal/transport"
	"github.com/postalsys/muti-metroo/internal/wizard/prompt"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Result contains the wizard output.
type Result struct {
	Config           *config.Config
	ConfigPath       string
	DataDir          string
	CertsDir         string
	ServiceInstalled bool
	EmbedConfig      bool   // Config embedded in binary instead of saved to file
	ServiceName      string // Custom service name (default: muti-metroo)
	EmbeddedBinary   string // Path to binary with embedded config (if EmbedConfig is true)
}

// Wizard manages the interactive setup process.
type Wizard struct {
	existingCfg      *config.Config // Loaded from existing config file or embedded binary
	targetBinaryPath string         // Path to binary to embed config into (if set)
}

// New creates a new setup wizard.
func New() *Wizard {
	return &Wizard{}
}

// SetTargetBinary sets a binary path to embed config into.
// If the binary already has embedded config, it will be loaded as defaults.
func (w *Wizard) SetTargetBinary(binaryPath string) error {
	w.targetBinaryPath = binaryPath

	// Check if the binary has embedded config
	has, err := embed.HasEmbeddedConfig(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to check binary for embedded config: %w", err)
	}

	if has {
		// Load embedded config as defaults
		data, err := embed.ReadEmbeddedConfig(binaryPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded config from binary: %w", err)
		}
		cfg, err := config.Parse(data)
		if err != nil {
			return fmt.Errorf("failed to parse embedded config: %w", err)
		}
		w.existingCfg = cfg
	}

	return nil
}

// Run executes the interactive setup wizard.
func (w *Wizard) Run() (*Result, error) {
	w.printBanner()

	// If a target binary was set, inform the user
	if w.targetBinaryPath != "" {
		if w.existingCfg != nil {
			prompt.PrintInfo(fmt.Sprintf("Editing embedded config from: %s", w.targetBinaryPath))
			fmt.Println("The existing embedded configuration will be used as defaults.")
		} else {
			prompt.PrintInfo(fmt.Sprintf("Target binary: %s", w.targetBinaryPath))
			fmt.Println("Configuration will be embedded into this binary.")
		}
		fmt.Println()
	}

	// Step 1: Basic setup
	dataDir, configPath, displayName, err := w.askBasicSetup()
	if err != nil {
		return nil, err
	}

	// Step 2: Agent role
	roles, err := w.askAgentRoles()
	if err != nil {
		return nil, err
	}

	// Step 3: Network configuration
	transport, listenAddr, listenPath, plainText, err := w.askNetworkConfig()
	if err != nil {
		return nil, err
	}

	// Step 4: TLS setup
	certsDir, tlsConfig, err := w.askTLSSetup(dataDir)
	if err != nil {
		return nil, err
	}

	// Step 5: Peer connections (if not standalone)
	peers, err := w.askPeerConnections(transport)
	if err != nil {
		return nil, err
	}

	// Step 6: SOCKS5 config (if ingress)
	var socks5Config config.SOCKS5Config
	if contains(roles, "ingress") {
		socks5Config, err = w.askSOCKS5Config()
		if err != nil {
			return nil, err
		}
	}

	// Step 7: Exit config (if exit)
	var exitConfig config.ExitConfig
	if contains(roles, "exit") {
		exitConfig, err = w.askExitConfig()
		if err != nil {
			return nil, err
		}
	}

	// Step 8: Advanced options
	healthEnabled, logLevel, err := w.askAdvancedOptions()
	if err != nil {
		return nil, err
	}

	// Step 9: Shell configuration
	shellConfig, err := w.askShellConfig()
	if err != nil {
		return nil, err
	}

	// Step 10: File transfer configuration
	fileTransferConfig, err := w.askFileTransferConfig()
	if err != nil {
		return nil, err
	}

	// Step 11: Management key encryption
	managementConfig, err := w.askManagementKey()
	if err != nil {
		return nil, err
	}

	// Step 12: Configuration delivery method
	// Skip this step if a target binary was specified - always embed to it
	var embedConfig bool
	var serviceName string
	if w.targetBinaryPath != "" {
		embedConfig = true
		// Extract service name from binary filename
		serviceName = filepath.Base(w.targetBinaryPath)
		if ext := filepath.Ext(serviceName); ext != "" {
			serviceName = serviceName[:len(serviceName)-len(ext)]
		}
	} else {
		embedConfig, serviceName, err = w.askConfigDelivery()
		if err != nil {
			return nil, err
		}
	}

	// Build configuration
	cfg := w.buildConfig(
		dataDir, displayName, transport, listenAddr, listenPath, plainText,
		tlsConfig, peers, socks5Config, exitConfig,
		healthEnabled, logLevel, shellConfig, fileTransferConfig, managementConfig,
	)

	// Initialize identity
	agentID, _, err := identity.LoadOrCreate(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize agent identity: %w", err)
	}

	// Initialize E2E encryption keypair
	keypair, created, err := identity.LoadOrCreateKeypair(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize E2E encryption keypair: %w", err)
	}
	if created {
		fmt.Println("\n[OK] Generated new E2E encryption keypair")
	}

	// Handle config delivery based on user choice
	var embeddedBinary string
	if embedConfig {
		if w.targetBinaryPath != "" {
			// Embed config to the specified target binary
			embeddedBinary, err = w.embedConfigToTargetBinary(cfg)
			if err != nil {
				return nil, err
			}
		} else {
			// Create new binary with embedded config
			embeddedBinary, err = w.embedConfigToBinary(cfg, serviceName)
			if err != nil {
				return nil, err
			}
		}
		// Also write config file for reference/backup
		if err := w.writeConfig(cfg, configPath); err != nil {
			prompt.PrintWarning(fmt.Sprintf("Could not save backup config file: %v", err))
		} else {
			fmt.Printf("Backup config saved to: %s\n", configPath)
		}
	} else {
		// Write configuration file (traditional mode)
		if err := w.writeConfig(cfg, configPath); err != nil {
			return nil, err
		}
	}

	// Print summary
	w.printSummary(agentID, keypair, configPath, cfg)

	// Step 13: Service installation (on supported platforms)
	// Skip if editing a target binary - it's likely already installed as a service
	var serviceInstalled bool
	if service.IsSupported() && w.targetBinaryPath == "" {
		if embedConfig {
			serviceInstalled, err = w.askServiceInstallationEmbedded(embeddedBinary, serviceName, dataDir)
		} else {
			serviceInstalled, err = w.askServiceInstallationWithName(configPath, serviceName)
		}
		if err != nil {
			return nil, err
		}
	}

	return &Result{
		Config:           cfg,
		ConfigPath:       configPath,
		DataDir:          dataDir,
		CertsDir:         certsDir,
		ServiceInstalled: serviceInstalled,
		EmbedConfig:      embedConfig,
		ServiceName:      serviceName,
		EmbeddedBinary:   embeddedBinary,
	}, nil
}

func (w *Wizard) printBanner() {
	prompt.PrintBanner("Muti Metroo Setup Wizard", "Userspace Mesh Networking Agent")
	fmt.Println()
}

func (w *Wizard) askBasicSetup() (dataDir, configPath, displayName string, err error) {
	dataDir = "./data"
	configPath = "./config.yaml"
	displayName = ""

	prompt.PrintHeader("Basic Setup", "Configure the essential paths for your agent.")

	// First, ask for config path so we can try to load existing config
	configPath, err = prompt.ReadLineValidated("Config File Path", "./config.yaml", func(s string) error {
		if s == "" {
			return fmt.Errorf("config path is required")
		}
		if !strings.HasSuffix(s, ".yaml") && !strings.HasSuffix(s, ".yml") {
			return fmt.Errorf("config file should have .yaml or .yml extension")
		}
		return nil
	})
	if err != nil {
		return
	}

	// Try to load existing config to use as defaults
	if existingCfg, loadErr := config.Load(configPath); loadErr == nil {
		w.existingCfg = existingCfg
		dataDir = existingCfg.Agent.DataDir
		displayName = existingCfg.Agent.DisplayName
		fmt.Println("\n  [INFO] Found existing configuration, using values as defaults.")
	}

	// Now ask for remaining settings with defaults from existing config
	dataDir, err = prompt.ReadLineValidated("Data Directory", dataDir, func(s string) error {
		if s == "" {
			return fmt.Errorf("data directory is required")
		}
		return nil
	})
	if err != nil {
		return
	}

	displayName, err = prompt.ReadLine("Display Name (press Enter to use Agent ID)", displayName)
	return
}

func (w *Wizard) askAgentRoles() ([]string, error) {
	var selectedIndices []int

	// Try to infer roles from existing config
	if w.existingCfg != nil {
		if w.existingCfg.SOCKS5.Enabled {
			selectedIndices = append(selectedIndices, 0) // ingress
		}
		if w.existingCfg.Exit.Enabled {
			selectedIndices = append(selectedIndices, 2) // exit
		}
		// If has peers but not ingress/exit, assume transit
		if len(w.existingCfg.Peers) > 0 && !w.existingCfg.SOCKS5.Enabled && !w.existingCfg.Exit.Enabled {
			selectedIndices = append(selectedIndices, 1) // transit
		}
		// Default to transit if nothing else
		if len(selectedIndices) == 0 {
			selectedIndices = append(selectedIndices, 1) // transit
		}
	}

	prompt.PrintHeader("Agent Role", "Select the roles this agent will perform.\nYou can select multiple roles.")

	options := []string{
		"Ingress (SOCKS5 proxy entry point)",
		"Transit (relay traffic between peers)",
		"Exit (connect to external networks)",
	}

	for {
		selectedIndices, err := prompt.MultiSelect("Select Roles", options, selectedIndices)
		if err != nil {
			return nil, err
		}

		if len(selectedIndices) == 0 {
			fmt.Println("  Error: select at least one role")
			continue
		}

		// Map indices to role strings
		roleMap := []string{"ingress", "transit", "exit"}
		var roles []string
		for _, idx := range selectedIndices {
			if idx >= 0 && idx < len(roleMap) {
				roles = append(roles, roleMap[idx])
			}
		}

		return roles, nil
	}
}

func (w *Wizard) askNetworkConfig() (transport, listenAddr, path string, plainText bool, err error) {
	transport = "quic"
	listenAddr = "0.0.0.0:4433"
	path = "/mesh"

	// Use existing config defaults if available
	if w.existingCfg != nil && len(w.existingCfg.Listeners) > 0 {
		l := w.existingCfg.Listeners[0]
		transport = l.Transport
		listenAddr = l.Address
		if l.Path != "" {
			path = l.Path
		}
		plainText = l.PlainText
	}

	prompt.PrintHeader("Network Configuration", "Configure how this agent listens for connections.")

	// Transport selection
	transportOptions := []string{
		"QUIC (UDP, fastest)",
		"HTTP/2 (TCP, firewall-friendly)",
		"WebSocket (TCP, proxy-friendly)",
	}

	fmt.Println("Transport Protocol (QUIC is recommended for best performance):")
	idx, err := prompt.Select("Select", transportOptions, transportIndex(transport))
	if err != nil {
		return
	}
	transport = transportValues[idx]

	// Listen address
	listenAddr, err = prompt.ReadLineValidated("Listen Address", listenAddr, func(s string) error {
		if s == "" {
			return fmt.Errorf("listen address is required")
		}
		_, _, err := net.SplitHostPort(s)
		if err != nil {
			return fmt.Errorf("invalid address format (use host:port)")
		}
		return nil
	})
	if err != nil {
		return
	}

	// Ask for path if using HTTP-based transport
	if transport == "h2" || transport == "ws" {
		path, err = prompt.ReadLineValidated("HTTP Path", path, func(s string) error {
			if s == "" || !strings.HasPrefix(s, "/") {
				return fmt.Errorf("path must start with /")
			}
			return nil
		})
		if err != nil {
			return
		}
	}

	// Ask about reverse proxy for WebSocket transport
	if transport == "ws" {
		plainText, err = prompt.Confirm(
			"Behind reverse proxy (TLS handled by Nginx/Caddy/Apache)?",
			plainText,
		)
		if err != nil {
			return
		}
		if plainText {
			fmt.Println("\n[OK] Listener will accept plain WebSocket (no TLS)")
			fmt.Println("    Your reverse proxy should handle TLS termination.")
		}
	}

	return
}

func (w *Wizard) askTLSSetup(dataDir string) (certsDir string, tlsConfig config.GlobalTLSConfig, err error) {
	certsDir = filepath.Join(dataDir, "certs")

	prompt.PrintHeader("TLS Configuration", "Muti Metroo uses E2E encryption (X25519 + ChaCha20-Poly1305) for security.\nTransport TLS is optional - certificates are auto-generated if not configured.")

	// Simplified certificate setup: 2 options only
	tlsOptions := []string{
		"Self-signed certificates (Recommended)",
		"Strict TLS with CA verification (Advanced)",
	}

	idx, err := prompt.Select("Certificate Setup", tlsOptions, 0)
	if err != nil {
		return
	}

	switch idx {
	case 0: // self-signed (auto-generate at startup)
		fmt.Println("\n[OK] TLS certificates will be auto-generated at startup")
		fmt.Println("    E2E encryption provides security - TLS verification is optional.")
		return certsDir, config.GlobalTLSConfig{}, nil

	case 1: // strict TLS with CA
		tlsConfig, err = w.askStrictTLSSetup()
		if err != nil {
			return
		}
	}

	return
}

// askStrictTLSSetup prompts for strict TLS configuration with CA verification.
func (w *Wizard) askStrictTLSSetup() (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Strict TLS Setup", "Configure CA-based TLS verification.\nAll certificates will be embedded in the config file.")

	strictOptions := []string{
		"Paste CA certificate, agent certificate, and key",
		"Generate from CA private key",
	}

	idx, err := prompt.Select("Setup method", strictOptions, 0)
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	switch idx {
	case 0:
		return w.pasteCACertAndAgentCert()
	case 1:
		return w.generateFromCAKey()
	}

	return config.GlobalTLSConfig{}, nil
}

// pasteCACertAndAgentCert prompts user to paste CA cert, agent cert, and key.
func (w *Wizard) pasteCACertAndAgentCert() (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Paste Certificates", "Paste your PEM-encoded CA certificate, agent certificate, and private key.\nAll will be embedded in the config file with strict mode enabled.")

	fmt.Println("CA Certificate (PEM):")
	caPEM, err := prompt.ReadMultiLine("CA Certificate (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(caPEM, "-----BEGIN CERTIFICATE-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid CA certificate format - must be PEM")
	}

	fmt.Println("\nAgent Certificate (PEM):")
	certPEM, err := prompt.ReadMultiLine("Agent Certificate (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(certPEM, "-----BEGIN CERTIFICATE-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid agent certificate format - must be PEM")
	}

	fmt.Println("\nAgent Private Key (PEM):")
	keyPEM, err := prompt.ReadMultiLine("Agent Private Key (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(keyPEM, "-----BEGIN") || !strings.Contains(keyPEM, "PRIVATE KEY-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid private key format - must be PEM")
	}

	// Validate the key is EC (not RSA)
	if _, err := certutil.ParsePrivateKeyPEM([]byte(keyPEM)); err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid private key: %w", err)
	}

	fmt.Println("\n[OK] Certificates validated and will be embedded in config with strict mode enabled")

	return config.GlobalTLSConfig{
		CAPEM:   caPEM,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Strict:  true,
	}, nil
}

// generateFromCAKey generates CA cert and agent cert from a CA private key.
func (w *Wizard) generateFromCAKey() (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Generate from CA Private Key", "Paste your CA private key (ECDSA P-256).\nThe wizard will derive the CA certificate and generate an agent certificate.")

	fmt.Println("CA Private Key (PEM):")
	caKeyPEM, err := prompt.ReadMultiLine("CA Private Key (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	// Parse the CA private key
	caKey, err := certutil.ParsePrivateKeyPEM([]byte(caKeyPEM))
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	// Ask for validity period
	validDaysStr, err := prompt.ReadLineValidated("Validity period (days)", "365", func(s string) error {
		if s == "" {
			return nil
		}
		d, err := strconv.Atoi(s)
		if err != nil || d < 1 {
			return fmt.Errorf("must be a positive number")
		}
		return nil
	})
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDays := 365
	if d, err := strconv.Atoi(validDaysStr); err == nil && d > 0 {
		validDays = d
	}

	// Ask for common name
	commonName, err := prompt.ReadLine("Agent Common Name", "muti-metroo")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	// Generate CA certificate from private key
	ca, err := certutil.GenerateCACertFromKey(caKey, commonName+" CA", validDays)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate CA certificate: %w", err)
	}

	// Generate agent certificate signed by CA
	validFor := time.Duration(validDays) * 24 * time.Hour
	opts := certutil.DefaultPeerOptions(commonName)
	opts.ValidFor = validFor
	opts.ParentCert = ca.Certificate
	opts.ParentKey = ca.PrivateKey
	opts.DNSNames = append(opts.DNSNames, "localhost")
	opts.IPAddresses = append(opts.IPAddresses, net.ParseIP("127.0.0.1"))

	agentCert, err := certutil.GenerateCert(opts)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate agent certificate: %w", err)
	}

	fmt.Println("\n[OK] Certificates generated from CA key")
	fmt.Printf("    CA Fingerprint:    %s\n", ca.Fingerprint())
	fmt.Printf("    Agent Fingerprint: %s\n", agentCert.Fingerprint())
	fmt.Println("    Certificates will be embedded in config with strict mode enabled")

	return config.GlobalTLSConfig{
		CAPEM:   string(ca.CertPEM),
		CertPEM: string(agentCert.CertPEM),
		KeyPEM:  string(agentCert.KeyPEM),
		Strict:  true,
	}, nil
}

func (w *Wizard) generateCertificates(certsDir string) (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Generate Certificates", "A CA and server certificate will be generated.")

	commonName, err := prompt.ReadLine("Common Name", "muti-metroo")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDaysStr, err := prompt.ReadLineValidated("Validity (days)", "365", func(s string) error {
		if s == "" {
			return nil
		}
		d, err := strconv.Atoi(s)
		if err != nil || d < 1 {
			return fmt.Errorf("must be a positive number")
		}
		return nil
	})
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDays := 365
	if d, err := strconv.Atoi(validDaysStr); err == nil && d > 0 {
		validDays = d
	}

	// Generate CA
	validFor := time.Duration(validDays) * 24 * time.Hour
	ca, err := certutil.GenerateCA(commonName+" CA", validFor)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate CA: %w", err)
	}

	caPath := filepath.Join(certsDir, "ca.crt")
	caKeyPath := filepath.Join(certsDir, "ca.key")
	if err := ca.SaveToFiles(caPath, caKeyPath); err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to save CA: %w", err)
	}

	// Generate server certificate
	opts := certutil.DefaultPeerOptions(commonName)
	opts.ValidFor = validFor
	opts.ParentCert = ca.Certificate
	opts.ParentKey = ca.PrivateKey
	opts.DNSNames = append(opts.DNSNames, "localhost")
	opts.IPAddresses = append(opts.IPAddresses, net.ParseIP("127.0.0.1"))

	cert, err := certutil.GenerateCert(opts)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate certificate: %w", err)
	}

	certPath := filepath.Join(certsDir, "server.crt")
	keyPath := filepath.Join(certsDir, "server.key")
	if err := cert.SaveToFiles(certPath, keyPath); err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to save certificate: %w", err)
	}

	fmt.Printf("\n[OK] Generated CA certificate: %s\n", caPath)
	fmt.Printf("[OK] Generated server certificate: %s\n", certPath)
	fmt.Printf("  Fingerprint: %s\n\n", cert.Fingerprint())

	return config.GlobalTLSConfig{
		Cert: certPath,
		Key:  keyPath,
		CA:   caPath,
	}, nil
}

func (w *Wizard) pasteCertificates(certsDir string) (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Paste Certificate", "Paste your PEM-encoded certificate content.\nInclude the BEGIN/END markers.")

	fmt.Println("Certificate (PEM) - paste server certificate:")
	certContent, err := prompt.ReadMultiLine("Certificate (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(certContent, "-----BEGIN CERTIFICATE-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid certificate format")
	}

	fmt.Println("Private Key (PEM) - paste private key:")
	keyContent, err := prompt.ReadMultiLine("Private Key (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(keyContent, "-----BEGIN") || !strings.Contains(keyContent, "PRIVATE KEY-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid private key format")
	}

	fmt.Println("CA Certificate (PEM) - optional, for peer verification and mTLS:")
	caContent, err := prompt.ReadMultiLine("CA Certificate (PEM) - Optional")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	// Write certificate files
	certPath := filepath.Join(certsDir, "server.crt")
	keyPath := filepath.Join(certsDir, "server.key")

	if err := os.WriteFile(certPath, []byte(certContent), 0644); err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to write certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(keyContent), 0600); err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to write key: %w", err)
	}

	tlsConfig := config.GlobalTLSConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	if caContent != "" && strings.Contains(caContent, "-----BEGIN CERTIFICATE-----") {
		caPath := filepath.Join(certsDir, "ca.crt")
		if err := os.WriteFile(caPath, []byte(caContent), 0644); err != nil {
			return config.GlobalTLSConfig{}, fmt.Errorf("failed to write CA: %w", err)
		}
		tlsConfig.CA = caPath
	}

	fmt.Printf("\n[OK] Saved certificate to: %s\n", certPath)
	fmt.Printf("[OK] Saved private key to: %s\n\n", keyPath)

	return tlsConfig, nil
}

// generateSelfSignedCert generates a self-signed certificate and embeds it in the config.
// No CA is generated - just a simple self-signed cert for transport encryption.
func (w *Wizard) generateSelfSignedCert() (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Generate Certificate", "A self-signed certificate will be generated and embedded in your config.")

	commonName, err := prompt.ReadLine("Common Name", "muti-metroo")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDaysStr, err := prompt.ReadLineValidated("Validity (days)", "365", func(s string) error {
		if s == "" {
			return nil
		}
		d, err := strconv.Atoi(s)
		if err != nil || d < 1 {
			return fmt.Errorf("must be a positive number")
		}
		return nil
	})
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDays := 365
	if d, err := strconv.Atoi(validDaysStr); err == nil && d > 0 {
		validDays = d
	}

	// Generate self-signed certificate using transport helper
	validFor := time.Duration(validDays) * 24 * time.Hour
	certPEM, keyPEM, err := transport.GenerateSelfSignedCert(commonName, validFor)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate certificate: %w", err)
	}

	fmt.Println("\n[OK] Certificate generated and will be embedded in config")

	return config.GlobalTLSConfig{
		CertPEM: string(certPEM),
		KeyPEM:  string(keyPEM),
	}, nil
}

// pasteCertificatesEmbedded stores pasted certificates as embedded PEM in config.
func (w *Wizard) pasteCertificatesEmbedded() (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Paste Certificate", "Paste your PEM-encoded certificate content.\nCertificates will be embedded in the config file.")

	fmt.Println("Certificate (PEM) - paste server certificate:")
	certContent, err := prompt.ReadMultiLine("Certificate (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(certContent, "-----BEGIN CERTIFICATE-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid certificate format")
	}

	fmt.Println("Private Key (PEM) - paste private key:")
	keyContent, err := prompt.ReadMultiLine("Private Key (PEM)")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}
	if !strings.Contains(keyContent, "-----BEGIN") || !strings.Contains(keyContent, "PRIVATE KEY-----") {
		return config.GlobalTLSConfig{}, fmt.Errorf("invalid private key format")
	}

	fmt.Println("CA Certificate (PEM) - optional, for strict TLS verification and mTLS:")
	caContent, err := prompt.ReadMultiLine("CA Certificate (PEM) - Optional")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	tlsConfig := config.GlobalTLSConfig{
		CertPEM: certContent,
		KeyPEM:  keyContent,
	}

	if caContent != "" && strings.Contains(caContent, "-----BEGIN CERTIFICATE-----") {
		tlsConfig.CAPEM = caContent
	}

	fmt.Println("\n[OK] Certificates will be embedded in config")

	return tlsConfig, nil
}

// setupStrictTLS generates a CA and agent certificate for strict TLS verification.
func (w *Wizard) setupStrictTLS() (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Strict TLS Setup", "This will generate a CA and agent certificate for strict TLS verification.\nPeer certificates will be validated against the CA.")

	commonName, err := prompt.ReadLine("Common Name", "muti-metroo")
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDaysStr, err := prompt.ReadLineValidated("Validity (days)", "365", func(s string) error {
		if s == "" {
			return nil
		}
		d, err := strconv.Atoi(s)
		if err != nil || d < 1 {
			return fmt.Errorf("must be a positive number")
		}
		return nil
	})
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	validDays := 365
	if d, err := strconv.Atoi(validDaysStr); err == nil && d > 0 {
		validDays = d
	}

	// Generate CA
	validFor := time.Duration(validDays) * 24 * time.Hour
	ca, err := certutil.GenerateCA(commonName+" CA", validFor)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate CA: %w", err)
	}

	// Generate agent certificate signed by CA
	opts := certutil.DefaultPeerOptions(commonName)
	opts.ValidFor = validFor
	opts.ParentCert = ca.Certificate
	opts.ParentKey = ca.PrivateKey
	opts.DNSNames = append(opts.DNSNames, "localhost")
	opts.IPAddresses = append(opts.IPAddresses, net.ParseIP("127.0.0.1"))

	cert, err := certutil.GenerateCert(opts)
	if err != nil {
		return config.GlobalTLSConfig{}, fmt.Errorf("failed to generate certificate: %w", err)
	}

	fmt.Println("\n[OK] CA and agent certificate generated")
	fmt.Printf("  CA Fingerprint: %s\n", ca.Fingerprint())
	fmt.Printf("  Agent Fingerprint: %s\n", cert.Fingerprint())
	fmt.Println("  Certificates will be embedded in config with strict mode enabled")

	return config.GlobalTLSConfig{
		CAPEM:   string(ca.CertPEM),
		CertPEM: string(cert.CertPEM),
		KeyPEM:  string(cert.KeyPEM),
		Strict:  true,
	}, nil
}

func (w *Wizard) useExistingCertificates(certsDir string) (config.GlobalTLSConfig, error) {
	prompt.PrintHeader("Existing Certificates", "Specify paths to your existing certificate files.")

	certPath, err := prompt.ReadLineValidated("Certificate File", filepath.Join(certsDir, "server.crt"), func(s string) error {
		if _, err := os.Stat(s); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", s)
		}
		return nil
	})
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	keyPath, err := prompt.ReadLineValidated("Private Key File", filepath.Join(certsDir, "server.key"), func(s string) error {
		if _, err := os.Stat(s); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", s)
		}
		return nil
	})
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	caPath, err := prompt.ReadLine("CA Certificate File (optional)", filepath.Join(certsDir, "ca.crt"))
	if err != nil {
		return config.GlobalTLSConfig{}, err
	}

	tlsConfig := config.GlobalTLSConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	if caPath != "" {
		if _, err := os.Stat(caPath); err == nil {
			tlsConfig.CA = caPath
		}
	}

	return tlsConfig, nil
}

func (w *Wizard) askPeerConnections(transport string) ([]config.PeerConfig, error) {
	prompt.PrintHeader("Peer Connections", "Configure connections to other mesh agents.")

	addPeers, err := prompt.Confirm("Add peer connections?", false)
	if err != nil {
		return nil, err
	}

	if !addPeers {
		return nil, nil
	}

	var peers []config.PeerConfig
	addMore := true

	for addMore {
		peer, err := w.askSinglePeer(transport, len(peers)+1)
		if err != nil {
			return nil, err
		}

		// Test connectivity to the peer
		fmt.Println()
		prompt.PrintInfo("Testing connectivity to peer...")
		if err := w.testPeerConnectivity(peer); err != nil {
			prompt.PrintWarning(fmt.Sprintf("Could not connect to peer: %v", err))
			prompt.PrintInfo("The listener may not be running yet, or a firewall may be blocking.")

			options := []string{
				"Continue anyway (I'll set up the listener later)",
				"Retry the connection test",
				"Re-enter peer configuration",
				"Skip this peer",
			}

			action, err := prompt.Select("What would you like to do?", options, 0)
			if err != nil {
				return nil, err
			}

			switch action {
			case 0: // Continue anyway
				peers = append(peers, peer)
			case 1: // Retry
				continue // Loop will re-test same peer
			case 2: // Re-enter
				continue // Will prompt for new peer config
			case 3: // Skip
				// Don't add peer, continue to "add another?" prompt
			}
		} else {
			prompt.PrintSuccess("Connected successfully!")
			peers = append(peers, peer)
		}

		addMore, err = prompt.Confirm("Add another peer?", false)
		if err != nil {
			return nil, err
		}
	}

	return peers, nil
}

// testPeerConnectivity tests if a peer is reachable.
func (w *Wizard) testPeerConnectivity(peer config.PeerConfig) error {
	// Determine strict verify - default to false (skip verification)
	strictVerify := false
	if peer.TLS.Strict != nil {
		strictVerify = *peer.TLS.Strict
	}

	opts := probe.Options{
		Transport:    peer.Transport,
		Address:      peer.Address,
		Path:         peer.Path,
		Timeout:      10 * time.Second,
		StrictVerify: strictVerify,
		CACert:       peer.TLS.CA,
		ClientCert:   peer.TLS.Cert,
		ClientKey:    peer.TLS.Key,
	}

	// Set default path if not specified
	if opts.Path == "" && (peer.Transport == "h2" || peer.Transport == "ws") {
		opts.Path = "/mesh"
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	result := probe.Probe(ctx, opts)
	if !result.Success {
		return fmt.Errorf("%s", result.ErrorDetail)
	}

	// Show remote agent info if available
	if result.RemoteDisplayName != "" {
		prompt.PrintInfo(fmt.Sprintf("Remote agent: %s (%s)", result.RemoteDisplayName, result.RemoteID[:12]))
	} else if result.RemoteID != "" {
		prompt.PrintInfo(fmt.Sprintf("Remote agent ID: %s", result.RemoteID[:12]))
	}
	prompt.PrintInfo(fmt.Sprintf("Round-trip time: %dms", result.RTT.Milliseconds()))

	return nil
}

func (w *Wizard) askSinglePeer(defaultTransport string, peerNum int) (config.PeerConfig, error) {
	peer := config.PeerConfig{
		Transport: defaultTransport,
	}

	prompt.PrintHeader(fmt.Sprintf("Peer #%d", peerNum), "")

	peerAddr, err := prompt.ReadLineValidated("Peer Address (host:port)", "", func(s string) error {
		if s == "" {
			return fmt.Errorf("address is required")
		}
		_, _, err := net.SplitHostPort(s)
		if err != nil {
			return fmt.Errorf("invalid address format")
		}
		return nil
	})
	if err != nil {
		return peer, err
	}
	peer.Address = peerAddr

	peerID, err := prompt.ReadLine("Expected Agent ID (hex string, or 'auto')", "auto")
	if err != nil {
		return peer, err
	}
	if peerID == "" || peerID == "auto" {
		peer.ID = "auto"
	} else {
		peer.ID = peerID
	}

	// Transport selection
	idx, err := prompt.Select("Transport", transportLabels, transportIndex(defaultTransport))
	if err != nil {
		return peer, err
	}
	peer.Transport = transportValues[idx]

	// Note: Certificate verification is OFF by default because E2E encryption provides security.
	// Only ask about strict mode for advanced users who want additional CA-based verification.
	enableStrict, err := prompt.Confirm("Enable strict TLS verification? (requires CA-signed certs)", false)
	if err != nil {
		return peer, err
	}
	if enableStrict {
		strictVal := true
		peer.TLS.Strict = &strictVal
	}

	// Ask for path if HTTP transport
	if peer.Transport == "h2" || peer.Transport == "ws" {
		peerPath, err := prompt.ReadLine("HTTP Path", "/mesh")
		if err != nil {
			return peer, err
		}
		peer.Path = peerPath
	}

	return peer, nil
}

func (w *Wizard) askSOCKS5Config() (config.SOCKS5Config, error) {
	cfg := config.SOCKS5Config{
		Enabled:        true,
		Address:        "127.0.0.1:1080",
		MaxConnections: 1000,
	}
	var enableAuth bool

	// Use existing config defaults if available
	if w.existingCfg != nil && w.existingCfg.SOCKS5.Address != "" {
		cfg.Address = w.existingCfg.SOCKS5.Address
		cfg.MaxConnections = w.existingCfg.SOCKS5.MaxConnections
		enableAuth = w.existingCfg.SOCKS5.Auth.Enabled
	}

	prompt.PrintHeader("SOCKS5 Proxy", "Configure the SOCKS5 ingress proxy.")

	var err error
	cfg.Address, err = prompt.ReadLineValidated("Listen Address", cfg.Address, func(s string) error {
		_, _, err := net.SplitHostPort(s)
		return err
	})
	if err != nil {
		return cfg, err
	}

	enableAuth, err = prompt.Confirm("Enable authentication?", enableAuth)
	if err != nil {
		return cfg, err
	}

	if enableAuth {
		username, err := prompt.ReadLineValidated("Username", "", func(s string) error {
			if s == "" {
				return fmt.Errorf("username required")
			}
			return nil
		})
		if err != nil {
			return cfg, err
		}

		password, err := prompt.ReadPassword("Password")
		if err != nil {
			return cfg, err
		}
		if password == "" {
			return cfg, fmt.Errorf("password required")
		}

		cfg.Auth.Enabled = true
		cfg.Auth.Users = []config.SOCKS5UserConfig{{
			Username: username,
			Password: password,
		}}
	}

	return cfg, nil
}

func (w *Wizard) askExitConfig() (config.ExitConfig, error) {
	cfg := config.ExitConfig{
		Enabled: true,
		DNS: config.DNSConfig{
			Servers: []string{"8.8.8.8:53", "1.1.1.1:53"},
			Timeout: 5 * time.Second,
		},
	}
	routesStr := "0.0.0.0/0\n::/0"

	// Use existing config defaults if available
	if w.existingCfg != nil && len(w.existingCfg.Exit.Routes) > 0 {
		routesStr = strings.Join(w.existingCfg.Exit.Routes, "\n")
		cfg.DNS = w.existingCfg.Exit.DNS
	}

	prompt.PrintHeader("Exit Node Configuration", "Configure this agent as an exit node.\nIt will allow traffic to specified networks.")

	fmt.Println("Allowed Routes (CIDR) - one CIDR per line (e.g., 0.0.0.0/0 for all traffic):")
	fmt.Println("Enter routes, one per line. Enter empty line to finish.")

	var routes []string
	for {
		line, err := prompt.ReadLine("Route (or empty to finish)", "")
		if err != nil {
			return cfg, err
		}
		if line == "" {
			break
		}
		line = strings.TrimSpace(line)
		if _, _, err := net.ParseCIDR(line); err != nil {
			fmt.Printf("  Invalid CIDR: %s\n", line)
			continue
		}
		routes = append(routes, line)
	}

	// Use defaults if no routes entered
	if len(routes) == 0 {
		for _, line := range strings.Split(routesStr, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				routes = append(routes, line)
			}
		}
	}

	cfg.Routes = routes

	// Domain routes (optional)
	fmt.Println()
	prompt.PrintHeader("Domain Routes (Optional)", "Route traffic by domain name.\nExamples: api.internal.corp, *.example.com")

	fmt.Println("Enter domain patterns, one per line. Enter empty line to finish.")
	fmt.Println("Use *.domain.tld for single-level wildcards.")

	// Use existing domain routes as hints
	if w.existingCfg != nil && len(w.existingCfg.Exit.DomainRoutes) > 0 {
		fmt.Printf("Current domain routes: %v\n", w.existingCfg.Exit.DomainRoutes)
	}

	var domainRoutes []string
	for {
		line, err := prompt.ReadLine("Domain (or empty to finish)", "")
		if err != nil {
			return cfg, err
		}
		if line == "" {
			break
		}
		line = strings.TrimSpace(line)
		if err := validateDomainPattern(line); err != nil {
			fmt.Printf("  Invalid pattern: %v\n", err)
			continue
		}
		domainRoutes = append(domainRoutes, line)
	}

	cfg.DomainRoutes = domainRoutes
	return cfg, nil
}

// validateDomainPattern validates a domain pattern for the wizard.
func validateDomainPattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("empty domain pattern")
	}

	// Check for wildcard pattern
	var baseDomain string
	if strings.HasPrefix(pattern, "*.") {
		baseDomain = pattern[2:]
	} else {
		baseDomain = pattern
	}

	if baseDomain == "" {
		return fmt.Errorf("empty domain after wildcard")
	}

	// Basic domain validation
	if strings.HasPrefix(baseDomain, ".") || strings.HasSuffix(baseDomain, ".") {
		return fmt.Errorf("domain cannot start or end with a dot")
	}

	if strings.Contains(baseDomain, "..") {
		return fmt.Errorf("domain cannot contain consecutive dots")
	}

	// Must have at least one dot (TLD)
	if !strings.Contains(baseDomain, ".") {
		return fmt.Errorf("domain must have at least one dot (e.g., example.com)")
	}

	return nil
}

func (w *Wizard) askAdvancedOptions() (healthEnabled bool, logLevel string, err error) {
	healthEnabled = true
	logLevel = "info"

	// Use existing config defaults if available
	if w.existingCfg != nil {
		healthEnabled = w.existingCfg.HTTP.Enabled
		logLevel = w.existingCfg.Agent.LogLevel
	}

	prompt.PrintHeader("Advanced Options", "Configure monitoring and logging.")

	// Log level selection
	logLevelOptions := []string{
		"Debug (verbose)",
		"Info (recommended)",
		"Warning",
		"Error (quiet)",
	}
	logLevelMap := []string{"debug", "info", "warn", "error"}

	defaultIdx := 1 // info
	for i, l := range logLevelMap {
		if l == logLevel {
			defaultIdx = i
			break
		}
	}

	idx, err := prompt.Select("Log Level", logLevelOptions, defaultIdx)
	if err != nil {
		return
	}
	logLevel = logLevelMap[idx]

	healthEnabled, err = prompt.Confirm("Enable health check endpoint? (HTTP endpoint for monitoring and CLI)", healthEnabled)
	return
}

func (w *Wizard) askShellConfig() (config.ShellConfig, error) {
	cfg := config.ShellConfig{
		Enabled:     false,
		Whitelist:   []string{},
		MaxSessions: 0, // 0 = unlimited
	}

	// Use existing config defaults if available
	if w.existingCfg != nil {
		cfg.Timeout = w.existingCfg.Shell.Timeout
		cfg.MaxSessions = w.existingCfg.Shell.MaxSessions
	}

	prompt.PrintHeader("Remote Shell Access", "Shell allows executing commands remotely on this agent.\nCommands must be whitelisted for security.")

	enableShell, err := prompt.Confirm("Enable Remote Shell? (requires authentication)", false)
	if err != nil {
		return cfg, err
	}

	if !enableShell {
		return cfg, nil
	}

	cfg.Enabled = true

	// Ask for password
	fmt.Println("\nSet a password to protect shell access.")
	fmt.Println("This password will be hashed and stored securely.")

	password, err := readConfirmedPassword("Shell Password", 8)
	if err != nil {
		return cfg, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return cfg, fmt.Errorf("failed to hash password: %w", err)
	}
	cfg.PasswordHash = string(hash)

	// Ask for whitelist
	prompt.PrintHeader("Command Whitelist", "Only whitelisted commands can be executed.\nFor security, the default is no commands allowed.")

	whitelistOptions := []string{
		"No commands (safest)",
		"Allow all commands (testing only!)",
		"Custom whitelist",
	}

	idx, err := prompt.Select("Whitelist Mode", whitelistOptions, 0)
	if err != nil {
		return cfg, err
	}

	switch idx {
	case 0: // none
		cfg.Whitelist = []string{}
	case 1: // all
		cfg.Whitelist = []string{"*"}
		fmt.Print("\n[WARNING] All commands are allowed! Use only for testing.\n\n")
	case 2: // custom
		fmt.Println("\nEnter allowed commands, one per line (e.g., whoami, ip, hostname, bash).")
		fmt.Println("Enter empty line to finish.")

		var commands []string
		for {
			cmd, err := prompt.ReadLine("Command (or empty to finish)", "")
			if err != nil {
				return cfg, err
			}
			if cmd == "" {
				break
			}
			commands = append(commands, strings.TrimSpace(cmd))
		}

		if len(commands) == 0 {
			return cfg, fmt.Errorf("at least one command is required for custom whitelist")
		}
		cfg.Whitelist = commands
	}

	return cfg, nil
}

func (w *Wizard) askFileTransferConfig() (config.FileTransferConfig, error) {
	cfg := config.FileTransferConfig{
		Enabled:      false,
		MaxFileSize:  500 * 1024 * 1024, // 500MB
		AllowedPaths: []string{},
	}

	// Use existing config defaults if available
	if w.existingCfg != nil {
		if w.existingCfg.FileTransfer.MaxFileSize > 0 {
			cfg.MaxFileSize = w.existingCfg.FileTransfer.MaxFileSize
		}
		cfg.AllowedPaths = w.existingCfg.FileTransfer.AllowedPaths
	}

	prompt.PrintHeader("File Transfer", "File transfer allows uploading and downloading files to/from this agent.\nFiles are transferred via the control channel.")

	enableFileTransfer, err := prompt.Confirm("Enable file transfer? (requires authentication)", false)
	if err != nil {
		return cfg, err
	}

	if !enableFileTransfer {
		return cfg, nil
	}

	cfg.Enabled = true

	// Ask for password
	fmt.Println("\nSet a password to protect file transfer access.")
	fmt.Println("This password will be hashed and stored securely.")

	password, err := readConfirmedPassword("File Transfer Password", 8)
	if err != nil {
		return cfg, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return cfg, fmt.Errorf("failed to hash password: %w", err)
	}
	cfg.PasswordHash = string(hash)

	// Ask for max file size
	maxSizeMB, err := prompt.ReadLineValidated("Max File Size (MB)", "500", func(s string) error {
		if s == "" {
			return nil
		}
		size, err := strconv.Atoi(s)
		if err != nil || size < 1 {
			return fmt.Errorf("must be a positive number")
		}
		return nil
	})
	if err != nil {
		return cfg, err
	}

	if size, err := strconv.Atoi(maxSizeMB); err == nil && size > 0 {
		cfg.MaxFileSize = int64(size) * 1024 * 1024
	}

	// Ask for path restrictions
	pathOptions := []string{
		"Allow all paths (no restrictions)",
		"Restrict to specific directories",
	}

	idx, err := prompt.Select("Path Access", pathOptions, 0)
	if err != nil {
		return cfg, err
	}

	if idx == 1 { // restricted
		fmt.Println("\nEnter allowed path prefixes, one per line (e.g., /tmp, /home/user/uploads).")
		fmt.Println("Enter empty line to finish.")

		var paths []string
		for {
			path, err := prompt.ReadLine("Path (or empty to finish)", "")
			if err != nil {
				return cfg, err
			}
			if path == "" {
				break
			}
			paths = append(paths, strings.TrimSpace(path))
		}

		if len(paths) == 0 {
			return cfg, fmt.Errorf("at least one path is required for restricted mode")
		}
		cfg.AllowedPaths = paths
	}

	return cfg, nil
}

func (w *Wizard) askManagementKey() (config.ManagementConfig, error) {
	cfg := config.ManagementConfig{}

	// Use existing config defaults if available
	if w.existingCfg != nil && w.existingCfg.Management.PublicKey != "" {
		// If there's an existing management key, offer to keep it
		prompt.PrintHeader("Management Key Encryption (OPSEC Protection)", "Existing management key configuration found.")

		keepExisting, err := prompt.Confirm(
			fmt.Sprintf("Keep existing management key? (current: %s...)", w.existingCfg.Management.PublicKey[:16]),
			true,
		)
		if err != nil {
			return cfg, err
		}

		if keepExisting {
			return w.existingCfg.Management, nil
		}
	}

	prompt.PrintHeader("Management Key Encryption (OPSEC Protection)",
		"Encrypt mesh topology data so only operators can view it.\nCompromised agents will only see encrypted blobs.\n\nThis is recommended for red team operations.")

	keyOptions := []string{
		"Skip (not recommended for red team ops)",
		"Generate new management keypair",
		"Enter existing public key",
	}

	idx, err := prompt.Select("Management Key Setup", keyOptions, 0)
	if err != nil {
		return cfg, err
	}

	switch idx {
	case 0: // skip
		return cfg, nil

	case 1: // generate
		keypair, err := identity.NewKeypair()
		if err != nil {
			return cfg, fmt.Errorf("failed to generate management keypair: %w", err)
		}

		cfg.PublicKey = hex.EncodeToString(keypair.PublicKey[:])

		// Ask if this is an operator node
		prompt.PrintHeader("Operator Node", "Operator nodes can view mesh topology data.\nField agents should NOT have the private key.")

		isOperator, err := prompt.Confirm("Is this an operator/management node?", false)
		if err != nil {
			return cfg, err
		}

		if isOperator {
			cfg.PrivateKey = hex.EncodeToString(keypair.PrivateKey[:])
		}

		// Always show the private key so operator can save it
		fmt.Println()
		fmt.Println("=== SAVE THIS MANAGEMENT KEYPAIR ===")
		fmt.Println()
		fmt.Println("Public Key (add to ALL agent configs):")
		fmt.Printf("  %s\n", hex.EncodeToString(keypair.PublicKey[:]))
		fmt.Println()
		fmt.Println("Private Key (add to OPERATOR config only, keep secure!):")
		fmt.Printf("  %s\n", hex.EncodeToString(keypair.PrivateKey[:]))
		fmt.Println()

		if isOperator {
			fmt.Println("[INFO] Private key will be included in this config.")
		} else {
			fmt.Println("[INFO] Private key NOT included in this config (field agent).")
		}
		fmt.Println()

	case 2: // existing
		pubKey, err := prompt.ReadLineValidated("Management Public Key (64-char hex)", "", func(s string) error {
			s = normalizeHexKey(s)
			if len(s) != 64 {
				return fmt.Errorf("public key must be 64 hex characters (got %d)", len(s))
			}
			if _, err := hex.DecodeString(s); err != nil {
				return fmt.Errorf("invalid hex string: %v", err)
			}
			return nil
		})
		if err != nil {
			return cfg, err
		}
		cfg.PublicKey = normalizeHexKey(pubKey)

		// Ask if this is an operator node with the private key
		hasPrivateKey, err := prompt.Confirm("Do you have the private key?", false)
		if err != nil {
			return cfg, err
		}

		if hasPrivateKey {
			privKey, err := prompt.ReadPassword("Management Private Key (64-char hex)")
			if err != nil {
				return cfg, err
			}
			privKey = normalizeHexKey(privKey)

			if len(privKey) != 64 {
				return cfg, fmt.Errorf("private key must be 64 hex characters (got %d)", len(privKey))
			}
			if _, err := hex.DecodeString(privKey); err != nil {
				return cfg, fmt.Errorf("invalid hex string: %v", err)
			}

			cfg.PrivateKey = privKey

			// Verify keys match
			privKeyBytes, _ := hex.DecodeString(privKey)
			var privKeyArr [32]byte
			copy(privKeyArr[:], privKeyBytes)
			derivedPub := identity.DerivePublicKey(privKeyArr)
			derivedPubHex := hex.EncodeToString(derivedPub[:])

			if derivedPubHex != cfg.PublicKey {
				return cfg, fmt.Errorf("private key does not match public key")
			}
		}
	}

	return cfg, nil
}

func (w *Wizard) buildConfig(
	dataDir, displayName, transport, listenAddr, listenPath string,
	plainText bool,
	tlsConfig config.GlobalTLSConfig,
	peers []config.PeerConfig,
	socks5Config config.SOCKS5Config,
	exitConfig config.ExitConfig,
	healthEnabled bool,
	logLevel string,
	shellConfig config.ShellConfig,
	fileTransferConfig config.FileTransferConfig,
	managementConfig config.ManagementConfig,
) *config.Config {
	cfg := config.Default()

	cfg.Agent.DataDir = dataDir
	cfg.Agent.DisplayName = displayName
	cfg.Agent.LogLevel = logLevel
	cfg.Agent.LogFormat = "text"

	// Global TLS config
	cfg.TLS = tlsConfig

	// Listener (uses global TLS config, no per-listener TLS needed)
	listener := config.ListenerConfig{
		Transport: transport,
		Address:   listenAddr,
		PlainText: plainText,
	}
	if transport == "h2" || transport == "ws" {
		listener.Path = listenPath
	}
	cfg.Listeners = []config.ListenerConfig{listener}

	// Peers
	cfg.Peers = peers

	// SOCKS5
	cfg.SOCKS5 = socks5Config

	// Exit
	cfg.Exit = exitConfig

	// HTTP
	cfg.HTTP.Enabled = healthEnabled
	if healthEnabled {
		cfg.HTTP.Address = ":8080"
	}

	// Shell
	cfg.Shell = shellConfig

	// File Transfer
	cfg.FileTransfer = fileTransferConfig

	// Management Key Encryption
	cfg.Management = managementConfig

	return cfg
}

func (w *Wizard) writeConfig(cfg *config.Config, path string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := `# Muti Metroo Configuration
# Generated by setup wizard
# See https://github.com/postalsys/muti-metroo for documentation

`
	if err := os.WriteFile(path, []byte(header+string(data)), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (w *Wizard) printSummary(agentID identity.AgentID, keypair *identity.Keypair, configPath string, cfg *config.Config) {
	fmt.Println()
	prompt.PrintDivider()
	fmt.Println("[OK] Setup Complete!")
	prompt.PrintDivider()
	fmt.Println()

	// Show display name if set, otherwise show agent ID
	if cfg.Agent.DisplayName != "" {
		fmt.Printf("  Display Name:   %s\n", cfg.Agent.DisplayName)
		fmt.Printf("  Agent ID:       %s\n", agentID.String())
	} else {
		fmt.Printf("  Agent ID:       %s\n", agentID.String())
	}
	fmt.Printf("  E2E Public Key: %s\n", keypair.PublicKeyString())
	fmt.Printf("  Config file:    %s\n", configPath)
	fmt.Printf("  Data dir:       %s\n", cfg.Agent.DataDir)
	fmt.Println()

	if len(cfg.Listeners) > 0 {
		l := cfg.Listeners[0]
		fmt.Printf("  Listener:     %s://%s\n", l.Transport, l.Address)
	}

	if cfg.SOCKS5.Enabled {
		fmt.Printf("  SOCKS5:       %s\n", cfg.SOCKS5.Address)
	}

	if cfg.Exit.Enabled {
		fmt.Printf("  Exit routes:  %v\n", cfg.Exit.Routes)
	}

	if cfg.HTTP.Enabled {
		fmt.Printf("  HTTP API:     http://%s\n", cfg.HTTP.Address)
	}

	if cfg.Shell.Enabled {
		fmt.Printf("  Shell:        enabled (%d commands whitelisted)\n", len(cfg.Shell.Whitelist))
		if len(cfg.Shell.Whitelist) == 1 && cfg.Shell.Whitelist[0] == "*" {
			fmt.Println("                [WARNING] All commands allowed!")
		}
	}

	if cfg.FileTransfer.Enabled {
		maxSizeMB := cfg.FileTransfer.MaxFileSize / (1024 * 1024)
		fmt.Printf("  File Transfer: enabled (max %d MB)\n", maxSizeMB)
		if len(cfg.FileTransfer.AllowedPaths) > 0 {
			fmt.Printf("                 restricted to: %v\n", cfg.FileTransfer.AllowedPaths)
		} else {
			fmt.Println("                 [WARNING] All paths allowed!")
		}
	}

	if cfg.Management.PublicKey != "" {
		if cfg.Management.PrivateKey != "" {
			fmt.Printf("  Management:   enabled (operator node, can decrypt)\n")
		} else {
			fmt.Printf("  Management:   enabled (field agent, encrypt only)\n")
		}
		fmt.Printf("                public key: %s...\n", cfg.Management.PublicKey[:16])
	}

	fmt.Println()
	fmt.Println("  To start the agent:")
	fmt.Printf("    muti-metroo run -c %s\n", configPath)
	fmt.Println()
}

// askServiceInstallationWithName handles service installation with a custom service name.
func (w *Wizard) askServiceInstallationWithName(configPath, serviceName string) (bool, error) {
	// Get absolute path for config
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute config path: %w", err)
	}

	cfg := service.DefaultConfig(absConfigPath)
	cfg.Name = serviceName
	cfg.DisplayName = serviceName + " Mesh Agent"

	// Handle Linux specially - offer user service option
	if runtime.GOOS == "linux" {
		return w.askLinuxServiceInstallationWithConfig(cfg, absConfigPath, false)
	}

	// macOS and Windows - system service only
	return w.askSystemServiceInstallation(cfg, serviceName, false, "")
}

// askServiceInstallationEmbedded handles service installation for embedded config binaries.
func (w *Wizard) askServiceInstallationEmbedded(embeddedBinaryPath, serviceName, dataDir string) (bool, error) {
	cfg := service.ServiceConfig{
		Name:        serviceName,
		DisplayName: serviceName + " Mesh Agent",
		Description: "Userspace mesh networking agent for virtual TCP tunnels",
		WorkingDir:  dataDir,
	}

	// Handle Linux specially
	if runtime.GOOS == "linux" {
		return w.askLinuxServiceInstallationEmbedded(cfg, embeddedBinaryPath)
	}

	// macOS and Windows - system service only
	return w.askSystemServiceInstallation(cfg, serviceName, true, embeddedBinaryPath)
}

// askSystemServiceInstallation handles macOS/Windows service installation.
func (w *Wizard) askSystemServiceInstallation(cfg service.ServiceConfig, serviceName string, embedded bool, embeddedBinaryPath string) (bool, error) {
	var platformName string
	var privilegeCmd string

	switch runtime.GOOS {
	case "darwin":
		platformName = "launchd service"
		privilegeCmd = "sudo"
	case "windows":
		platformName = "Windows service"
		privilegeCmd = "Run as Administrator"
	default:
		return false, nil
	}

	// If not running as root/admin, show instructions instead
	if !service.IsRoot() {
		prompt.PrintHeader("Service Installation",
			fmt.Sprintf("To install as a %s, elevated privileges are required.\nYou can run the service install command later with %s.", platformName, privilegeCmd))

		showInstructions, err := prompt.Confirm("Show installation command?", true)
		if err != nil {
			return false, err
		}

		if showInstructions {
			fmt.Println()
			fmt.Println("To install as a service, run:")
			if embedded {
				installPath := service.GetInstallPath(serviceName)
				fmt.Printf("  1. Copy %s to %s\n", embeddedBinaryPath, installPath)
				switch runtime.GOOS {
				case "darwin":
					fmt.Printf("  2. sudo muti-metroo service install -n %s --embedded\n", serviceName)
				case "windows":
					fmt.Printf("  2. muti-metroo service install -n %s --embedded\n", serviceName)
					fmt.Println("     (Run this command as Administrator)")
				}
			} else {
				switch runtime.GOOS {
				case "darwin":
					fmt.Printf("  sudo muti-metroo service install -n %s -c %s\n", serviceName, cfg.ConfigPath)
				case "windows":
					fmt.Printf("  muti-metroo service install -n %s -c %s\n", serviceName, cfg.ConfigPath)
					fmt.Println("  (Run this command as Administrator)")
				}
			}
			fmt.Println()
		}

		return false, nil
	}

	prompt.PrintHeader("Service Installation",
		fmt.Sprintf("You are running as %s.\nWould you like to install %s as a %s?\n\nThe service will start automatically on boot.", w.privilegeLevel(), serviceName, platformName))

	if embedded {
		installPath := service.GetInstallPath(serviceName)
		fmt.Printf("Binary will be installed to: %s\n\n", installPath)
	}

	installService, err := prompt.Confirm(fmt.Sprintf("Install as %s?", platformName), false)
	if err != nil {
		return false, err
	}

	if !installService {
		return false, nil
	}

	fmt.Println()
	fmt.Printf("Installing %s...\n", platformName)

	var installErr error
	if embedded {
		installErr = service.InstallWithEmbedded(cfg, embeddedBinaryPath)
	} else {
		installErr = service.Install(cfg)
	}

	if installErr != nil {
		fmt.Printf("\n[WARNING] Failed to install service: %v\n", installErr)
		fmt.Println("You can install the service manually later.")
		return false, nil
	}

	fmt.Println()
	prompt.PrintSuccess(fmt.Sprintf("Installed as %s", platformName))

	switch runtime.GOOS {
	case "darwin":
		fmt.Println("\nUseful commands:")
		fmt.Printf("  sudo launchctl list | grep %s   # Check if running\n", serviceName)
		fmt.Printf("  tail -f /var/log/%s.log        # View logs\n", serviceName)
		fmt.Printf("  sudo launchctl stop com.%s     # Stop service\n", serviceName)
		fmt.Printf("  sudo launchctl start com.%s    # Start service\n", serviceName)
		fmt.Printf("  muti-metroo service uninstall -n %s  # Remove service\n", serviceName)
	case "windows":
		fmt.Println("\nUseful commands:")
		fmt.Printf("  sc query %s            # Check status\n", serviceName)
		fmt.Printf("  net start %s           # Start service\n", serviceName)
		fmt.Printf("  net stop %s            # Stop service\n", serviceName)
		fmt.Printf("  muti-metroo service uninstall -n %s  # Remove service\n", serviceName)
	}
	fmt.Println()

	return true, nil
}

// askLinuxServiceInstallationWithConfig handles Linux service installation with custom config.
func (w *Wizard) askLinuxServiceInstallationWithConfig(cfg service.ServiceConfig, absConfigPath string, _ bool) (bool, error) {
	isRoot := service.IsRoot()

	if isRoot {
		// Root user: offer choice between systemd and cron+nohup
		prompt.PrintHeader("Service Installation",
			fmt.Sprintf("You are running as root.\nChoose how to install %s as a service.\n\nThe service will start automatically on boot.", cfg.Name))

		options := []string{
			"systemd (recommended)",
			"Don't install as service",
		}
		choice, err := prompt.Select("Installation method:", options, 0)
		if err != nil {
			return false, err
		}

		if choice == 1 { // Don't install
			return false, nil
		}

		// systemd installation
		fmt.Println()
		fmt.Println("Installing systemd service...")

		installErr := service.Install(cfg)
		if installErr != nil {
			fmt.Printf("\n[WARNING] Failed to install service: %v\n", installErr)
			fmt.Println("You can install the service manually later.")
			return false, nil
		}

		fmt.Println()
		prompt.PrintSuccess("Installed as systemd service")
		fmt.Println("\nUseful commands:")
		fmt.Printf("  systemctl status %s    # Check status\n", cfg.Name)
		fmt.Printf("  journalctl -u %s -f    # View logs\n", cfg.Name)
		fmt.Printf("  systemctl restart %s   # Restart service\n", cfg.Name)
		fmt.Printf("  muti-metroo service uninstall -n %s  # Remove service\n", cfg.Name)
		fmt.Println()
		return true, nil
	}

	// Non-root user: show instructions
	prompt.PrintHeader("Service Installation",
		"To install as a systemd service, elevated privileges are required.\nRun the wizard with sudo for systemd installation.")

	showInstructions, err := prompt.Confirm("Show installation command?", true)
	if err != nil {
		return false, err
	}

	if showInstructions {
		fmt.Println()
		fmt.Println("To install later:")
		fmt.Printf("  sudo muti-metroo service install -n %s -c %s\n", cfg.Name, absConfigPath)
		fmt.Println()
	}

	return false, nil
}

// askLinuxServiceInstallationEmbedded handles Linux service installation for embedded config.
func (w *Wizard) askLinuxServiceInstallationEmbedded(cfg service.ServiceConfig, embeddedBinaryPath string) (bool, error) {
	isRoot := service.IsRoot()

	if isRoot {
		// Root user: systemd only for embedded
		prompt.PrintHeader("Service Installation",
			fmt.Sprintf("You are running as root.\nWould you like to install %s as a systemd service?\n\nThe service will start automatically on boot.", cfg.Name))

		installPath := service.GetInstallPath(cfg.Name)
		fmt.Printf("Binary will be installed to: %s\n\n", installPath)

		installService, err := prompt.Confirm("Install as systemd service?", false)
		if err != nil {
			return false, err
		}

		if !installService {
			return false, nil
		}

		// systemd installation with embedded binary
		fmt.Println()
		fmt.Println("Installing systemd service...")

		installErr := service.InstallWithEmbedded(cfg, embeddedBinaryPath)
		if installErr != nil {
			fmt.Printf("\n[WARNING] Failed to install service: %v\n", installErr)
			fmt.Println("You can install the service manually later.")
			return false, nil
		}

		fmt.Println()
		prompt.PrintSuccess("Installed as systemd service")
		fmt.Println("\nUseful commands:")
		fmt.Printf("  systemctl status %s    # Check status\n", cfg.Name)
		fmt.Printf("  journalctl -u %s -f    # View logs\n", cfg.Name)
		fmt.Printf("  systemctl restart %s   # Restart service\n", cfg.Name)
		fmt.Printf("  muti-metroo service uninstall -n %s  # Remove service\n", cfg.Name)
		fmt.Println()
		return true, nil
	}

	// Non-root user: show instructions
	prompt.PrintHeader("Service Installation",
		"To install as a systemd service, elevated privileges are required.\nRun the wizard with sudo for systemd installation.")

	showInstructions, err := prompt.Confirm("Show installation command?", true)
	if err != nil {
		return false, err
	}

	if showInstructions {
		installPath := service.GetInstallPath(cfg.Name)
		fmt.Println()
		fmt.Println("To install later:")
		fmt.Printf("  1. sudo cp %s %s\n", embeddedBinaryPath, installPath)
		fmt.Printf("  2. sudo muti-metroo service install -n %s --embedded\n", cfg.Name)
		fmt.Println()
	}

	return false, nil
}

// askConfigDelivery asks how to deploy the configuration.
// Returns whether to embed config and the custom service name.
func (w *Wizard) askConfigDelivery() (embedConfig bool, serviceName string, err error) {
	prompt.PrintHeader("Configuration Delivery",
		"Choose how to deploy the configuration.\n"+
			"Embedding creates a single-file binary with config baked in.")

	options := []string{
		"Save to config file (traditional)",
		"Embed in binary (single-file deployment)",
	}

	idx, err := prompt.Select("Delivery method", options, 0)
	if err != nil {
		return false, "", err
	}

	embedConfig = (idx == 1)

	// Ask for custom service name
	fmt.Println()
	prompt.PrintInfo("Service name is used when installing as a system service.")
	serviceName, err = prompt.ReadLine("Service name", "muti-metroo")
	if err != nil {
		return false, "", err
	}

	// Validate service name (alphanumeric and hyphens only)
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "muti-metroo"
	}

	// Simple validation: alphanumeric, hyphens, underscores
	for _, r := range serviceName {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false, "", fmt.Errorf("invalid service name: only alphanumeric characters, hyphens, and underscores are allowed")
		}
	}

	return embedConfig, serviceName, nil
}

// embedConfigToBinary creates a binary with embedded configuration.
// Returns the path to the created binary.
func (w *Wizard) embedConfigToBinary(cfg *config.Config, serviceName string) (string, error) {
	prompt.PrintHeader("Embedding Configuration",
		"The configuration will be XOR-obfuscated and appended to a copy of the binary.")

	// Marshal config to YAML
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	// Get current executable
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Determine default output path based on platform
	var outputPath string
	switch runtime.GOOS {
	case "windows":
		outputPath = filepath.Join(os.TempDir(), serviceName+".exe")
	default:
		outputPath = filepath.Join(os.TempDir(), serviceName)
	}

	// Ask for output path
	outputPath, err = prompt.ReadLine("Output binary path", outputPath)
	if err != nil {
		return "", err
	}

	// Check if output file already exists
	if _, err := os.Stat(outputPath); err == nil {
		overwrite, err := prompt.Confirm("File exists. Overwrite?", false)
		if err != nil {
			return "", err
		}
		if !overwrite {
			return "", fmt.Errorf("aborted: output file exists")
		}
	}

	// Create the embedded binary
	if err := embed.AppendConfig(execPath, outputPath, yamlData); err != nil {
		return "", fmt.Errorf("failed to embed config: %w", err)
	}

	fmt.Println()
	prompt.PrintSuccess(fmt.Sprintf("Created binary with embedded config: %s", outputPath))
	fmt.Println("This binary can be run without a config file.")

	return outputPath, nil
}

// embedConfigToTargetBinary embeds config into the specified target binary.
// This is used when the user provides another binary as the "config file" to update.
func (w *Wizard) embedConfigToTargetBinary(cfg *config.Config) (string, error) {
	prompt.PrintHeader("Embedding Configuration to Target Binary",
		fmt.Sprintf("The configuration will be embedded into: %s", w.targetBinaryPath))

	// Marshal config to YAML
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	binaryPath := w.targetBinaryPath

	// Create a temporary file for the new binary
	tmpFile, err := os.CreateTemp(filepath.Dir(binaryPath), ".muti-metroo-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Check if the target binary has embedded config already
	hasConfig, _ := embed.HasEmbeddedConfig(binaryPath)
	if hasConfig {
		// Copy the original binary without embedded config to temp file
		if err := embed.CopyBinaryWithoutConfig(binaryPath, tmpPath); err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("failed to extract original binary: %w", err)
		}
	} else {
		// Copy the binary as-is
		if err := copyFile(binaryPath, tmpPath); err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("failed to copy binary: %w", err)
		}
	}

	// Create the final binary with embedded config
	finalPath := binaryPath
	if err := embed.AppendConfig(tmpPath, finalPath, yamlData); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to embed config: %w", err)
	}

	os.Remove(tmpPath)

	fmt.Println()
	prompt.PrintSuccess(fmt.Sprintf("Embedded config into: %s", binaryPath))
	fmt.Println("Restart the service to apply changes.")

	return binaryPath, nil
}

// copyFile copies a file, preserving permissions.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcStat.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.ReadFrom(srcFile)
	return err
}

func (w *Wizard) privilegeLevel() string {
	switch runtime.GOOS {
	case "linux":
		return "root"
	case "windows":
		return "Administrator"
	default:
		return "elevated privileges"
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Transport options and their display labels for selection prompts.
var (
	transportLabels = []string{"QUIC", "HTTP/2", "WebSocket"}
	transportValues = []string{"quic", "h2", "ws"}
)

// transportIndex returns the index of the given transport in transportValues.
// Returns 0 (QUIC) if not found.
func transportIndex(transport string) int {
	for i, t := range transportValues {
		if t == transport {
			return i
		}
	}
	return 0
}

// normalizeHexKey removes common prefixes and whitespace from hex strings.
func normalizeHexKey(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return s
}

// readConfirmedPassword prompts for a password with confirmation.
// Returns the password or an error. Retries until passwords match and meet minimum length.
func readConfirmedPassword(promptLabel string, minLength int) (string, error) {
	for {
		password, err := prompt.ReadPassword(fmt.Sprintf("%s (min %d chars)", promptLabel, minLength))
		if err != nil {
			return "", err
		}
		if len(password) < minLength {
			fmt.Printf("  Error: password must be at least %d characters\n", minLength)
			continue
		}

		confirmPassword, err := prompt.ReadPassword("Confirm Password")
		if err != nil {
			return "", err
		}
		if password != confirmPassword {
			fmt.Println("  Error: passwords do not match")
			continue
		}
		return password, nil
	}
}
