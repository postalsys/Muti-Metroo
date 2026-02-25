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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/postalsys/muti-metroo/internal/certutil"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/embed"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/probe"
	"github.com/postalsys/muti-metroo/internal/service"
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

	// Step 1/11: Basic setup
	dataDir, configPath, displayName, err := w.askBasicSetup()
	if err != nil {
		return nil, err
	}

	// Step 2/11: Agent role
	roles, err := w.askAgentRoles()
	if err != nil {
		return nil, err
	}

	// Determine if transit-only (used to skip shell/file transfer)
	transitOnly := len(roles) == 1 && roles[0] == "transit"

	// Step 3/11: Network configuration
	transport, listenAddr, listenPath, plainText, err := w.askNetworkConfig()
	if err != nil {
		return nil, err
	}

	// Step 4/11: TLS setup
	certsDir, tlsConfig, err := w.askTLSSetup(dataDir)
	if err != nil {
		return nil, err
	}

	// Step 5/11: Peer connections
	peers, err := w.askPeerConnections(transport)
	if err != nil {
		return nil, err
	}

	// Step 6/11: SOCKS5 config (if ingress)
	var socks5Config config.SOCKS5Config
	if slices.Contains(roles, "ingress") {
		socks5Config, err = w.askSOCKS5Config()
		if err != nil {
			return nil, err
		}
	}

	// Step 7/11: Exit config (if exit)
	var exitConfig config.ExitConfig
	if slices.Contains(roles, "exit") {
		exitConfig, err = w.askExitConfig()
		if err != nil {
			return nil, err
		}
	}

	// Step 8/11: Monitoring & Logging (includes HTTP auth)
	healthEnabled, logLevel, httpTokenHash, err := w.askMonitoringAndLogging()
	if err != nil {
		return nil, err
	}

	// Step 9/11: Shell configuration (skip for transit-only)
	var shellConfig config.ShellConfig
	if !transitOnly {
		shellConfig, err = w.askShellConfig()
		if err != nil {
			return nil, err
		}
	}

	// Step 10/11: File transfer configuration (skip for transit-only)
	var fileTransferConfig config.FileTransferConfig
	if !transitOnly {
		fileTransferConfig, err = w.askFileTransferConfig()
		if err != nil {
			return nil, err
		}
	}

	// Step 11/11: Management key encryption
	managementConfig, err := w.askManagementKey()
	if err != nil {
		return nil, err
	}

	// Configuration delivery method
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
		healthEnabled, logLevel, httpTokenHash, shellConfig, fileTransferConfig, managementConfig,
	)

	// Initialize identity - when embedding to target binary, don't create files
	var agentID identity.AgentID
	var keypair *identity.Keypair

	if w.targetBinaryPath != "" {
		// Embedding to target binary - try to preserve existing identity or generate in-memory
		if w.existingCfg != nil && w.existingCfg.Agent.ID != "" && w.existingCfg.Agent.PrivateKey != "" {
			// Use existing identity from embedded config
			agentID, err = identity.ParseAgentID(w.existingCfg.Agent.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to parse existing agent ID: %w", err)
			}
			keypair, err = identity.KeypairFromConfig(w.existingCfg.Agent.PrivateKey, "")
			if err != nil {
				return nil, fmt.Errorf("failed to parse existing keypair: %w", err)
			}
			fmt.Println("\n[OK] Preserving existing identity from embedded config")
		} else {
			// Generate new identity in-memory (no files created)
			agentID, err = identity.NewAgentID()
			if err != nil {
				return nil, fmt.Errorf("failed to generate agent identity: %w", err)
			}
			keypair, err = identity.NewKeypair()
			if err != nil {
				return nil, fmt.Errorf("failed to generate E2E encryption keypair: %w", err)
			}
			fmt.Println("\n[OK] Generated new identity (in-memory, no files created)")
		}
	} else {
		// Traditional mode - use file-based identity
		agentID, _, err = identity.LoadOrCreate(dataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize agent identity: %w", err)
		}

		var created bool
		keypair, created, err = identity.LoadOrCreateKeypair(dataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize E2E encryption keypair: %w", err)
		}
		if created {
			fmt.Println("\n[OK] Generated new E2E encryption keypair")
		}
	}

	// If embedding config, always embed identity for true single-file deployment
	if embedConfig {
		// Set default action so binary auto-starts without "run" argument
		cfg.DefaultAction = "run"

		// Set identity values in config
		cfg.Agent.ID = agentID.String()
		cfg.Agent.PrivateKey = identity.KeyToString(keypair.PrivateKey)
		// Clear data_dir since identity is now in config
		cfg.Agent.DataDir = ""
		fmt.Println("[OK] Identity embedded in config - no data folder needed")
		fmt.Println("[OK] Default action set to 'run' - binary starts without arguments")
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
			// No backup config file when embedding to target binary
		} else {
			// Create new binary with embedded config
			embeddedBinary, err = w.embedConfigToBinary(cfg, serviceName)
			if err != nil {
				return nil, err
			}
			// Also write config file for reference/backup
			if err := w.writeConfig(cfg, configPath); err != nil {
				prompt.PrintWarning(fmt.Sprintf("Could not save backup config file: %v", err))
			} else {
				fmt.Printf("Backup config saved to: %s\n", configPath)
			}
		}
	} else {
		// Write configuration file (traditional mode)
		if err := w.writeConfig(cfg, configPath); err != nil {
			return nil, err
		}
	}

	// Print summary
	w.printSummary(agentID, keypair, configPath, cfg)

	// Service installation (on supported platforms)
	var serviceInstalled bool
	if service.IsSupported() {
		if w.targetBinaryPath != "" {
			// Target binary mode: offer service update if already installed
			serviceInstalled, err = w.askServiceInstallationTargetBinary(w.targetBinaryPath, serviceName, dataDir)
		} else if embedConfig {
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
	fmt.Println("Tip: You can re-run this wizard on an existing config to modify settings.")
	fmt.Println()
}

func (w *Wizard) askBasicSetup() (dataDir, configPath, displayName string, err error) {
	dataDir = "./data"
	configPath = "./config.yaml"
	displayName = ""

	// When embedding to a target binary, skip config path and data dir questions
	// Config will be embedded, and identity will be generated in-memory
	if w.targetBinaryPath != "" {
		prompt.PrintHeader("Step 1/11: Basic Setup", "Configure the agent display name.")

		// Use existing config defaults if available (from embedded config)
		if w.existingCfg != nil {
			displayName = w.existingCfg.Agent.DisplayName
		}

		displayName, err = prompt.ReadLine("Display Name (press Enter to use Agent ID)", displayName)
		return
	}

	prompt.PrintHeader("Step 1/11: Basic Setup", "Configure the essential paths for your agent.")

	// First, ask for config path so we can try to load existing config
	configPath, err = prompt.ReadLineValidated("Config File Path", "./config.yaml", func(s string) error {
		if s == "" {
			return fmt.Errorf("config path is required")
		}
		if !strings.HasSuffix(s, ".yaml") && !strings.HasSuffix(s, ".yml") {
			return fmt.Errorf("config file should have .yaml or .yml extension")
		}
		// Validate parent directory exists
		parentDir := filepath.Dir(s)
		if parentDir != "." {
			if info, statErr := os.Stat(parentDir); statErr != nil || !info.IsDir() {
				return fmt.Errorf("parent directory does not exist: %s", parentDir)
			}
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
		prompt.PrintInfo("Found existing configuration, using values as defaults.")
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
	fmt.Println("  Data directory stores agent identity and encryption keys.")

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
	} else {
		// Default to Transit when no existing config
		selectedIndices = []int{1}
	}

	prompt.PrintHeader("Step 2/11: Agent Role", "Select the roles this agent will perform.\nYou can select multiple roles.")

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

	prompt.PrintHeader("Step 3/11: Network Configuration", "Configure how this agent listens for connections.\n\nChoose based on your network:")

	// Transport selection
	transportOptions := []string{
		"QUIC - Best performance, requires UDP port access",
		"HTTP/2 - Works through firewalls that only allow TCP/443",
		"WebSocket - Works through HTTP proxies and CDNs",
	}

	idx, err := prompt.Select("Transport Protocol", transportOptions, transportIndex(transport))
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

	// Ask for path and reverse proxy if using HTTP-based transport
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

		plainText, err = prompt.Confirm(
			"Is TLS terminated by a reverse proxy? (e.g., Nginx, Caddy, Apache, Cloudflare)",
			plainText,
		)
		if err != nil {
			return
		}
		if plainText {
			prompt.PrintSuccess("Listener will accept plain connections (no TLS).")
			fmt.Println("    Your reverse proxy should handle TLS termination.")
		}
	}

	return
}

func (w *Wizard) askTLSSetup(dataDir string) (certsDir string, tlsConfig config.GlobalTLSConfig, err error) {
	certsDir = filepath.Join(dataDir, "certs")

	prompt.PrintHeader("Step 4/11: TLS Configuration",
		"All traffic is end-to-end encrypted (X25519 + ChaCha20-Poly1305).\n"+
			"Transport TLS adds certificate-based peer verification on top of E2E encryption.")

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
		prompt.PrintSuccess("TLS certificates will be auto-generated at startup.")
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
	default:
		return w.generateFromCAKey()
	}
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

	prompt.PrintSuccess("Certificates validated and will be embedded in config with strict mode enabled")

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

	prompt.PrintSuccess("Certificates generated from CA key")
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


func (w *Wizard) askPeerConnections(listenerTransport string) ([]config.PeerConfig, error) {
	prompt.PrintHeader("Step 5/11: Peer Connections", "Configure connections to other mesh agents.")

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
		peer, err := w.askSinglePeer(listenerTransport, len(peers)+1)
		if err != nil {
			return nil, err
		}

		// Test connectivity to the peer - use inner loop for retry
		for {
			fmt.Println()
			prompt.PrintInfo("Testing connectivity to peer...")
			if testErr := w.testPeerConnectivity(peer); testErr != nil {
				prompt.PrintWarning(fmt.Sprintf("Could not connect: %v", testErr))
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
				case 1: // Retry - re-test same peer config
					continue // Inner loop retries same peer
				case 2: // Re-enter
					peer, err = w.askSinglePeer(listenerTransport, len(peers)+1)
					if err != nil {
						return nil, err
					}
					continue // Inner loop tests newly entered peer
				case 3: // Skip
					// Don't add peer, continue to "add another?" prompt
				}
			} else {
				prompt.PrintSuccess("Connected successfully!")
				peers = append(peers, peer)
			}
			break // Exit inner retry loop
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

func (w *Wizard) askSinglePeer(listenerTransport string, peerNum int) (config.PeerConfig, error) {
	peer := config.PeerConfig{
		Transport: listenerTransport,
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

	// Pin to specific agent ID (behind confirmation)
	pinToID, err := prompt.Confirm("Pin to specific agent ID?", false)
	if err != nil {
		return peer, err
	}
	if pinToID {
		peerID, err := prompt.ReadLineValidated("Agent ID (hex string)", "", func(s string) error {
			if s == "" {
				return fmt.Errorf("agent ID is required")
			}
			return nil
		})
		if err != nil {
			return peer, err
		}
		peer.ID = peerID
	} else {
		peer.ID = "auto"
	}

	// Transport selection - default to listener's transport
	useSameTransport, err := prompt.Confirm(
		fmt.Sprintf("Use same transport as listener (%s)?", transportLabels[transportIndex(listenerTransport)]),
		true,
	)
	if err != nil {
		return peer, err
	}
	if !useSameTransport {
		idx, err := prompt.Select("Transport", transportLabels, transportIndex(listenerTransport))
		if err != nil {
			return peer, err
		}
		peer.Transport = transportValues[idx]
	}

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

	prompt.PrintHeader("Step 6/11: SOCKS5 Proxy", "Configure the SOCKS5 ingress proxy.")

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

		fmt.Println("  Note: SOCKS5 password is stored in plaintext (protocol requirement).")

		password, err := readConfirmedPassword("SOCKS5 Password", 8)
		if err != nil {
			return cfg, err
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

	prompt.PrintHeader("Step 7/11: Exit Node Configuration", "Configure this agent as an exit node.\nIt will allow traffic to specified networks.")

	// Show existing/default routes before prompting
	fmt.Println("Allowed Routes (CIDR):")
	if w.existingCfg != nil && len(w.existingCfg.Exit.Routes) > 0 {
		fmt.Printf("  Current routes: %s (enter empty line to keep)\n", strings.Join(w.existingCfg.Exit.Routes, ", "))
	} else {
		fmt.Println("  Default: 0.0.0.0/0 and ::/0 (enter empty line to keep defaults)")
	}
	fmt.Println()

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
	fmt.Println("Route traffic by domain name. Examples: api.internal.corp, *.example.com")
	fmt.Println("Use *.domain.tld for single-level wildcards.")

	// Use existing domain routes as hints
	if w.existingCfg != nil && len(w.existingCfg.Exit.DomainRoutes) > 0 {
		fmt.Printf("Current domain routes: %v\n", w.existingCfg.Exit.DomainRoutes)
	}
	fmt.Println()

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

// askMonitoringAndLogging handles log level, HTTP management API, and HTTP auth.
// This replaces the former askAdvancedOptions() and askHTTPAuth() as a single step.
func (w *Wizard) askMonitoringAndLogging() (healthEnabled bool, logLevel string, httpTokenHash string, err error) {
	healthEnabled = true
	logLevel = "info"

	// Use existing config defaults if available
	if w.existingCfg != nil {
		healthEnabled = w.existingCfg.HTTP.Enabled
		logLevel = w.existingCfg.Agent.LogLevel
	}

	prompt.PrintHeader("Step 8/11: Monitoring & Logging", "Configure monitoring, logging, and the HTTP management API.")

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

	idx, selectErr := prompt.Select("Log Level", logLevelOptions, defaultIdx)
	if selectErr != nil {
		err = selectErr
		return
	}
	logLevel = logLevelMap[idx]

	healthEnabled, err = prompt.Confirm("Enable HTTP management API? (health checks, dashboard, CLI remote commands)", healthEnabled)
	if err != nil {
		return
	}

	// HTTP API authentication (only if HTTP is enabled)
	if healthEnabled {
		var enableAuth bool
		enableAuth, err = prompt.Confirm("Enable HTTP API authentication?", false)
		if err != nil {
			return
		}

		if enableAuth {
			fmt.Println("\n  Protect the HTTP API with a bearer token.")
			fmt.Println("  When enabled, all non-health endpoints require authentication.")
			fmt.Println("  Use this token with: --token <token> or MUTI_METROO_TOKEN=<token>")

			var token string
			token, err = readConfirmedPassword("API Token", 8)
			if err != nil {
				return
			}

			var hash []byte
			hash, err = bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
			if err != nil {
				err = fmt.Errorf("failed to hash token: %w", err)
				return
			}
			httpTokenHash = string(hash)
		}
	}

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

	prompt.PrintHeader("Step 9/11: Remote Shell Access", "Shell allows executing commands remotely on this agent.")

	enableShell, err := prompt.Confirm("Enable Remote Shell?", false)
	if err != nil {
		return cfg, err
	}

	if !enableShell {
		return cfg, nil
	}

	cfg.Enabled = true

	// Ask for whitelist first (before optional password)
	fmt.Println("\n  Only whitelisted commands can be executed.")

	whitelistOptions := []string{
		"Allow all commands",
		"Custom whitelist",
		"No commands (lockdown - configure later via config file)",
	}

	idx, err := prompt.Select("Whitelist Mode", whitelistOptions, 0)
	if err != nil {
		return cfg, err
	}

	switch idx {
	case 0: // all
		cfg.Whitelist = []string{"*"}
	case 1: // custom
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
	case 2: // none
		cfg.Whitelist = []string{}
	}

	// Optional password
	protectWithPassword, err := prompt.Confirm("Protect shell with a password?", false)
	if err != nil {
		return cfg, err
	}

	if protectWithPassword {
		fmt.Println("  This password will be hashed and stored securely.")

		password, err := readConfirmedPassword("Shell Password", 8)
		if err != nil {
			return cfg, err
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return cfg, fmt.Errorf("failed to hash password: %w", err)
		}
		cfg.PasswordHash = string(hash)
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

	prompt.PrintHeader("Step 10/11: File Transfer",
		"File transfer allows uploading and downloading files to/from this agent.\n"+
			"Files are streamed directly through the mesh network.")

	enableFileTransfer, err := prompt.Confirm("Enable file transfer?", false)
	if err != nil {
		return cfg, err
	}

	if !enableFileTransfer {
		return cfg, nil
	}

	cfg.Enabled = true

	// Ask for max file size
	defaultSize := fmt.Sprintf("%d", cfg.MaxFileSize/(1024*1024))
	maxSizeMB, err := prompt.ReadLineValidated("Max File Size (MB, 0 = unlimited)", defaultSize, func(s string) error {
		if s == "" {
			return nil
		}
		size, err := strconv.Atoi(s)
		if err != nil || size < 0 {
			return fmt.Errorf("must be a non-negative number")
		}
		return nil
	})
	if err != nil {
		return cfg, err
	}

	if size, err := strconv.Atoi(maxSizeMB); err == nil {
		if size == 0 {
			cfg.MaxFileSize = 0
		} else {
			cfg.MaxFileSize = int64(size) * 1024 * 1024
		}
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
		fmt.Println("\nEnter allowed paths, one per line.")
		fmt.Println("Examples: /tmp, /home/user/uploads, C:\\Data")
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

	// Optional password
	protectWithPassword, err := prompt.Confirm("Protect file transfer with a password?", false)
	if err != nil {
		return cfg, err
	}

	if protectWithPassword {
		fmt.Println("  This password will be hashed and stored securely.")

		password, err := readConfirmedPassword("File Transfer Password", 8)
		if err != nil {
			return cfg, err
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return cfg, fmt.Errorf("failed to hash password: %w", err)
		}
		cfg.PasswordHash = string(hash)
	}

	return cfg, nil
}

func (w *Wizard) askManagementKey() (config.ManagementConfig, error) {
	cfg := config.ManagementConfig{}

	// Use existing config defaults if available
	if w.existingCfg != nil && w.existingCfg.Management.PublicKey != "" {
		// If there's an existing management key, offer to keep it
		prompt.PrintHeader("Step 11/11: Management Key Encryption", "Existing management key configuration found.")

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

	prompt.PrintHeader("Step 11/11: Management Key Encryption",
		"Encrypt mesh topology data so only operators can view it.\n"+
			"Compromised agents will only see encrypted blobs.\n"+
			"Recommended for sensitive deployments.")

	keyOptions := []string{
		"Generate new management keypair",
		"Enter existing key",
		"Skip",
	}

	idx, err := prompt.Select("Management Key Setup", keyOptions, 1) // Default: Enter existing key
	if err != nil {
		return cfg, err
	}

	switch idx {
	case 0: // generate
		return w.generateManagementKey()
	case 1: // enter existing key
		return w.enterExistingManagementKey()
	default: // skip
		return cfg, nil
	}
}

// generateManagementKey generates a new management keypair.
func (w *Wizard) generateManagementKey() (config.ManagementConfig, error) {
	cfg := config.ManagementConfig{}

	keypair, err := identity.NewKeypair()
	if err != nil {
		return cfg, fmt.Errorf("failed to generate management keypair: %w", err)
	}

	cfg.PublicKey = hex.EncodeToString(keypair.PublicKey[:])

	// Ask if this is an operator node
	isOperator, err := prompt.Confirm("Is this an operator/management node?", false)
	if err != nil {
		return cfg, err
	}

	if isOperator {
		cfg.PrivateKey = hex.EncodeToString(keypair.PrivateKey[:])
	}

	// Always show public key
	fmt.Println()
	fmt.Println("Public Key (add to ALL agent configs):")
	fmt.Printf("  %s\n", hex.EncodeToString(keypair.PublicKey[:]))

	// Private key display behind confirmation
	showPrivKey, err := prompt.Confirm("Display private key on screen?", true)
	if err != nil {
		return cfg, err
	}

	if showPrivKey {
		fmt.Println()
		fmt.Println("Private Key (add to OPERATOR config only, keep secure!):")
		fmt.Printf("  %s\n", hex.EncodeToString(keypair.PrivateKey[:]))
	} else {
		fmt.Println()
		fmt.Println("Private key generated but not displayed.")
		if isOperator {
			fmt.Println("It is included in the config file.")
		}
	}

	if isOperator {
		prompt.PrintInfo("Private key will be included in this config.")
	} else {
		prompt.PrintInfo("Private key NOT included in this config (field agent).")
	}
	fmt.Println()

	return cfg, nil
}

// enterExistingManagementKey prompts for an existing management key.
func (w *Wizard) enterExistingManagementKey() (config.ManagementConfig, error) {
	cfg := config.ManagementConfig{}

	keyTypeOptions := []string{
		"Private key (operator/management node)",
		"Public key (field agent)",
		"I don't have a key yet (generate new)",
	}

	idx, err := prompt.Select("Which key do you have?", keyTypeOptions, 0)
	if err != nil {
		return cfg, err
	}

	switch idx {
	case 0: // Private key - derive public key automatically
		privKey, err := prompt.ReadPassword("Management Private Key (64-char hex)")
		if err != nil {
			return cfg, err
		}
		privKey = normalizeHexKey(privKey)

		if len(privKey) != 64 {
			return cfg, fmt.Errorf("private key must be 64 hex characters (got %d)", len(privKey))
		}
		privKeyBytes, err := hex.DecodeString(privKey)
		if err != nil {
			return cfg, fmt.Errorf("invalid hex string: %v", err)
		}

		// Derive public key from private key
		var privKeyArr [32]byte
		copy(privKeyArr[:], privKeyBytes)
		derivedPub := identity.DerivePublicKey(privKeyArr)

		cfg.PrivateKey = privKey
		cfg.PublicKey = hex.EncodeToString(derivedPub[:])

		fmt.Printf("  Derived public key: %s\n", cfg.PublicKey)
		prompt.PrintSuccess("Both keys configured (operator node)")

	case 1: // Public key only
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
		prompt.PrintSuccess("Public key configured (field agent)")

	case 2: // Don't have a key - redirect to generate
		return w.generateManagementKey()
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
	httpTokenHash string,
	shellConfig config.ShellConfig,
	fileTransferConfig config.FileTransferConfig,
	managementConfig config.ManagementConfig,
) *config.Config {
	// Start with empty config - runtime applies defaults via config.Parse()
	cfg := &config.Config{}

	// Agent settings (always include data_dir)
	cfg.Agent.DataDir = dataDir
	if displayName != "" {
		cfg.Agent.DisplayName = displayName
	}
	if logLevel != "" && logLevel != "info" {
		cfg.Agent.LogLevel = logLevel
	}

	// Global TLS config (only if non-empty)
	if tlsConfig.Cert != "" || tlsConfig.CertPEM != "" || tlsConfig.Strict {
		cfg.TLS = tlsConfig
	}

	// Listener
	listener := config.ListenerConfig{
		Transport: transport,
		Address:   listenAddr,
	}
	if transport == "h2" || transport == "ws" {
		listener.Path = listenPath
	}
	if plainText {
		listener.PlainText = true
	}
	cfg.Listeners = []config.ListenerConfig{listener}

	// Peers (only if configured)
	if len(peers) > 0 {
		cfg.Peers = peers
	}

	// SOCKS5 (only if enabled)
	if socks5Config.Enabled {
		cfg.SOCKS5 = socks5Config
	}

	// Exit (only if enabled)
	if exitConfig.Enabled {
		cfg.Exit = exitConfig
	}

	// HTTP (only if enabled)
	if healthEnabled {
		cfg.HTTP.Enabled = true
		cfg.HTTP.Address = ":8080"
		if httpTokenHash != "" {
			cfg.HTTP.TokenHash = httpTokenHash
		}
	}

	// Shell (only if enabled)
	if shellConfig.Enabled {
		cfg.Shell = shellConfig
	}

	// File Transfer (only if enabled)
	if fileTransferConfig.Enabled {
		cfg.FileTransfer = fileTransferConfig
	}

	// Management Key Encryption (only if configured)
	if managementConfig.PublicKey != "" {
		cfg.Management = managementConfig
	}

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
		fmt.Printf("  Display Name:    %s\n", cfg.Agent.DisplayName)
		fmt.Printf("  Agent ID:        %s\n", agentID.String())
	} else {
		fmt.Printf("  Agent ID:        %s\n", agentID.String())
	}
	fmt.Printf("  E2E Public Key:  %s\n", keypair.PublicKeyString())

	// When embedding to target binary, don't show config/data paths (they're embedded)
	if w.targetBinaryPath == "" {
		fmt.Printf("  Config file:     %s\n", configPath)
		if cfg.Agent.DataDir != "" {
			fmt.Printf("  Data dir:        %s\n", cfg.Agent.DataDir)
		}
	}
	fmt.Println()

	if len(cfg.Listeners) > 0 {
		l := cfg.Listeners[0]
		fmt.Printf("  Listener:        %s://%s\n", l.Transport, l.Address)
	}

	if cfg.SOCKS5.Enabled {
		fmt.Printf("  SOCKS5:          %s\n", cfg.SOCKS5.Address)
	}

	if cfg.Exit.Enabled {
		fmt.Printf("  Exit routes:     %v\n", cfg.Exit.Routes)
	}

	if cfg.HTTP.Enabled {
		fmt.Printf("  HTTP API:        http://%s\n", cfg.HTTP.Address)
	}

	if cfg.Shell.Enabled {
		if len(cfg.Shell.Whitelist) == 1 && cfg.Shell.Whitelist[0] == "*" {
			fmt.Println("  Shell:           enabled (all commands)")
		} else {
			fmt.Printf("  Shell:           enabled (%d commands whitelisted)\n", len(cfg.Shell.Whitelist))
		}
	}

	if cfg.FileTransfer.Enabled {
		if cfg.FileTransfer.MaxFileSize == 0 {
			fmt.Println("  File Transfer:   enabled (unlimited)")
		} else {
			maxSizeMB := cfg.FileTransfer.MaxFileSize / (1024 * 1024)
			fmt.Printf("  File Transfer:   enabled (max %d MB)\n", maxSizeMB)
		}
	}

	if cfg.Management.PublicKey != "" {
		if cfg.Management.PrivateKey != "" {
			fmt.Println("  Management:      enabled (operator node)")
		} else {
			fmt.Println("  Management:      enabled (field agent)")
		}
	}

	fmt.Println()

	// Show appropriate start instructions based on deployment type
	if w.targetBinaryPath != "" {
		// DLL or binary with embedded config
		if strings.HasSuffix(strings.ToLower(w.targetBinaryPath), ".dll") {
			fmt.Println("  To start the agent:")
			fmt.Printf("    rundll32.exe %s,Run\n", w.targetBinaryPath)
		} else {
			fmt.Println("  To start the agent:")
			fmt.Printf("    %s\n", w.targetBinaryPath)
		}
	} else {
		fmt.Println("  To start the agent:")
		fmt.Printf("    muti-metroo run -c %s\n", configPath)
	}
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

	// Check if service already exists
	if updated, err := w.offerServiceUpdate(serviceName, ""); err != nil {
		return false, err
	} else if updated {
		return true, nil
	}

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

	// Check if service already exists
	if updated, err := w.offerServiceUpdate(serviceName, embeddedBinaryPath); err != nil {
		return false, err
	} else if updated {
		return true, nil
	}

	// Handle Linux specially
	if runtime.GOOS == "linux" {
		return w.askLinuxServiceInstallationEmbedded(cfg, embeddedBinaryPath)
	}

	// macOS and Windows - system service only
	return w.askSystemServiceInstallation(cfg, serviceName, true, embeddedBinaryPath)
}

// askServiceInstallationTargetBinary handles service installation when editing a target binary.
func (w *Wizard) askServiceInstallationTargetBinary(binaryPath, serviceName, dataDir string) (bool, error) {
	// For target binary mode, always check for existing service and offer update
	if updated, err := w.offerServiceUpdate(serviceName, binaryPath); err != nil {
		return false, err
	} else if updated {
		return true, nil
	}

	// If no existing service, offer fresh install
	cfg := service.ServiceConfig{
		Name:        serviceName,
		DisplayName: serviceName + " Mesh Agent",
		Description: "Userspace mesh networking agent for virtual TCP tunnels",
		WorkingDir:  dataDir,
	}

	// Handle Linux specially
	if runtime.GOOS == "linux" {
		return w.askLinuxServiceInstallationEmbedded(cfg, binaryPath)
	}

	// macOS and Windows - system service only
	return w.askSystemServiceInstallation(cfg, serviceName, true, binaryPath)
}

// offerServiceUpdate checks if a service is already installed and offers to update it.
// Returns (true, nil) if the service was updated, (false, nil) if user declined or not installed.
func (w *Wizard) offerServiceUpdate(serviceName, newBinaryPath string) (bool, error) {
	isSystemInstalled := service.IsInstalled(serviceName)
	isUserInstalled := service.IsUserInstalled(serviceName)

	if !isSystemInstalled && !isUserInstalled {
		return false, nil
	}

	prompt.PrintHeader("Service Installation", fmt.Sprintf("Service '%s' is already installed.", serviceName))

	doUpdate, err := prompt.Confirm("Update binary and restart service?", true)
	if err != nil {
		return false, err
	}

	if !doUpdate {
		return false, nil
	}

	// Determine the binary to install
	binaryPath := newBinaryPath
	if binaryPath == "" {
		// Use the current executable
		binaryPath, err = os.Executable()
		if err != nil {
			return false, fmt.Errorf("failed to get executable path: %w", err)
		}
		binaryPath, err = filepath.EvalSymlinks(binaryPath)
		if err != nil {
			return false, fmt.Errorf("failed to resolve executable path: %w", err)
		}
	}

	fmt.Println()

	if isUserInstalled {
		// User service: stop, copy, start
		fmt.Println("Stopping user service...")
		if err := service.StopUser(serviceName); err != nil {
			prompt.PrintWarning(fmt.Sprintf("Could not stop user service: %v", err))
		}

		fmt.Println("Updating binary...")
		if err := service.UpdateServiceBinary(serviceName, binaryPath); err != nil {
			return false, fmt.Errorf("failed to update binary: %w", err)
		}

		fmt.Println("Starting user service...")
		if err := service.StartUser(serviceName); err != nil {
			prompt.PrintWarning(fmt.Sprintf("Could not start user service: %v (may need manual start)", err))
		}
	} else {
		// System service: stop, copy, start
		fmt.Println("Stopping service...")
		if err := service.StopService(serviceName); err != nil {
			prompt.PrintWarning(fmt.Sprintf("Could not stop service: %v", err))
		}

		fmt.Println("Updating binary...")
		if err := service.UpdateServiceBinary(serviceName, binaryPath); err != nil {
			return false, fmt.Errorf("failed to update binary: %w", err)
		}

		fmt.Println("Starting service...")
		if err := service.StartService(serviceName); err != nil {
			prompt.PrintWarning(fmt.Sprintf("Could not start service: %v (may need manual start)", err))
		}
	}

	fmt.Println()
	prompt.PrintSuccess("Service updated and restarted.")
	return true, nil
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

	// If not running as root/admin, offer alternatives
	if !service.IsRoot() {
		// Windows non-admin: offer Registry Run key option
		if runtime.GOOS == "windows" {
			return w.askWindowsUserServiceInstallation(cfg, serviceName, embedded, embeddedBinaryPath)
		}

		// macOS: show instructions for sudo
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
				fmt.Printf("  2. sudo muti-metroo service install -n %s --embedded\n", serviceName)
			} else {
				fmt.Printf("  sudo muti-metroo service install -n %s -c %s\n", serviceName, cfg.ConfigPath)
			}
			fmt.Println()
		}

		return false, nil
	}

	prompt.PrintHeader("Service Installation",
		fmt.Sprintf("You are running as %s.\nWould you like to install %s as a %s?\n\nThe service will start automatically on boot.", w.privilegeLevel(), serviceName, platformName))

	installPath := service.GetInstallPath(serviceName)
	fmt.Printf("Binary will be installed to: %s\n\n", installPath)

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
		// For traditional config, also deploy the binary to system location
		installErr = service.InstallWithDeployment(cfg)
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

// askWindowsUserServiceInstallation handles Windows user service installation via Registry Run key.
func (w *Wizard) askWindowsUserServiceInstallation(cfg service.ServiceConfig, serviceName string, embedded bool, embeddedBinaryPath string) (bool, error) {
	prompt.PrintHeader("Service Installation",
		"You are not running as Administrator.")

	options := []string{
		"Install as user service (Registry Run)",
		"Show Windows Service instructions",
		"Skip service installation",
	}

	choice, err := prompt.Select("Choose an option:", options, 0)
	if err != nil {
		return false, err
	}

	switch choice {
	case 0: // User service via Registry Run key
		return w.installWindowsUserService(cfg, serviceName)

	case 1: // Show instructions for admin
		fmt.Println()
		fmt.Println("To install as a Windows Service, run as Administrator:")
		if embedded {
			installPath := service.GetInstallPath(serviceName)
			fmt.Printf("  1. Copy %s to %s\n", embeddedBinaryPath, installPath)
			fmt.Printf("  2. muti-metroo service install -n %s --embedded\n", serviceName)
		} else {
			fmt.Printf("  muti-metroo service install -n %s -c %s\n", serviceName, cfg.ConfigPath)
		}
		fmt.Println()
		return false, nil

	case 2: // Skip
		return false, nil
	}

	return false, nil
}

// installWindowsUserService installs the user service via Registry Run key.
func (w *Wizard) installWindowsUserService(cfg service.ServiceConfig, serviceName string) (bool, error) {
	prompt.PrintHeader("Windows User Service Setup",
		"This will create a Registry Run entry that starts the DLL at user logon.\n\n"+
			"Requirements:\n"+
			"  - muti-metroo.dll file\n"+
			"  - config.yaml file (or embedded config DLL)")

	// Ask for DLL path
	var dllPath string
	var err error

	// Try to find DLL in common locations
	defaultDLL := ""
	possiblePaths := []string{
		"muti-metroo.dll",
		"./muti-metroo.dll",
		"./build/muti-metroo.dll",
	}
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			defaultDLL = p
			break
		}
	}

	dllPath, err = prompt.ReadLineValidated("Path to muti-metroo.dll", defaultDLL, func(s string) error {
		if s == "" {
			return fmt.Errorf("DLL path is required")
		}
		if _, err := os.Stat(s); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", s)
		}
		if !strings.HasSuffix(strings.ToLower(s), ".dll") {
			return fmt.Errorf("file must be a .dll")
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	// Get absolute path
	absDLLPath, err := filepath.Abs(dllPath)
	if err != nil {
		return false, fmt.Errorf("failed to resolve DLL path: %w", err)
	}

	// Get config path - use cfg.ConfigPath if available
	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath, err = prompt.ReadLineValidated("Path to config file", "./config.yaml", func(s string) error {
			if s == "" {
				return fmt.Errorf("config path is required")
			}
			if _, err := os.Stat(s); os.IsNotExist(err) {
				return fmt.Errorf("file not found: %s", s)
			}
			return nil
		})
		if err != nil {
			return false, err
		}
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return false, fmt.Errorf("failed to resolve config path: %w", err)
	}

	// Confirm installation
	fmt.Println()
	fmt.Printf("DLL:    %s\n", absDLLPath)
	fmt.Printf("Config: %s\n", absConfigPath)
	fmt.Println()

	confirm, err := prompt.Confirm("Install user service?", true)
	if err != nil {
		return false, err
	}
	if !confirm {
		return false, nil
	}

	// Install the service
	fmt.Println()
	fmt.Println("Installing user service...")

	if err := service.InstallUserWindows(serviceName, absDLLPath, absConfigPath); err != nil {
		fmt.Printf("\n[WARNING] Failed to install user service: %v\n", err)
		fmt.Println("You can install the service manually with:")
		fmt.Printf("  muti-metroo service install --user -n \"%s\" --dll \"%s\" -c \"%s\"\n", serviceName, absDLLPath, absConfigPath)
		return false, nil
	}

	fmt.Println()
	prompt.PrintSuccess("User service installed and started!")
	fmt.Println("\nThe service is now running and will start automatically at user logon.")
	fmt.Println("\nManage the service with:")
	fmt.Println("  muti-metroo service status")
	fmt.Println("  muti-metroo service uninstall")

	return true, nil
}

// askLinuxServiceInstallationWithConfig handles Linux service installation with custom config.
func (w *Wizard) askLinuxServiceInstallationWithConfig(cfg service.ServiceConfig, absConfigPath string, _ bool) (bool, error) {
	isRoot := service.IsRoot()

	if isRoot {
		// Root user: offer choice between systemd and skip
		prompt.PrintHeader("Service Installation",
			fmt.Sprintf("You are running as root.\nChoose how to install %s as a service.\n\nThe service will start automatically on boot.", cfg.Name))

		installPath := service.GetInstallPath(cfg.Name)
		fmt.Printf("Binary will be installed to: %s\n\n", installPath)

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

		// systemd installation with binary deployment
		fmt.Println()
		fmt.Println("Installing systemd service...")

		installErr := service.InstallWithDeployment(cfg)
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

	// Non-root user: offer user service or show systemd instructions
	prompt.PrintHeader("Service Installation",
		"You are not running as root.")

	options := []string{
		"Install as user service (cron + nohup)",
		"Show systemd instructions",
		"Skip service installation",
	}

	choice, err := prompt.Select("Choose an option:", options, 0)
	if err != nil {
		return false, err
	}

	switch choice {
	case 0: // User service
		fmt.Println()
		fmt.Println("Installing user service...")

		if err := service.InstallUser(cfg); err != nil {
			fmt.Printf("\n[WARNING] Failed to install user service: %v\n", err)
			fmt.Println("You can install the service manually later.")
			return false, nil
		}

		fmt.Println()
		prompt.PrintSuccess("Installed as user service (cron + nohup)")
		fmt.Println("\nThe agent will start automatically at login.")
		fmt.Println("\nUseful commands:")
		fmt.Println("  muti-metroo service status   # Check status")
		fmt.Println("  muti-metroo service uninstall  # Remove service")
		fmt.Println()
		return true, nil

	case 1: // Show systemd instructions
		fmt.Println()
		fmt.Println("To install as systemd service later:")
		fmt.Printf("  sudo muti-metroo service install -n %s -c %s\n", cfg.Name, absConfigPath)
		fmt.Println()
		return false, nil

	default: // Skip
		return false, nil
	}
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
		"Choose how to deploy the configuration.")

	options := []string{
		"Save to config file (traditional)",
		"Embed in binary (single-file deployment)",
	}

	idx, err := prompt.Select("Delivery method", options, 0)
	if err != nil {
		return false, "", err
	}

	embedConfig = idx == 1

	// Ask for custom service name
	fmt.Println()
	serviceName, err = prompt.ReadLineValidated("Service name", "muti-metroo", func(s string) error {
		if s == "" {
			return fmt.Errorf("service name is required")
		}
		for _, r := range s {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
				return fmt.Errorf("only alphanumeric characters, hyphens, and underscores are allowed")
			}
		}
		return nil
	})
	if err != nil {
		return false, "", err
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
	if runtime.GOOS == "windows" {
		return "Administrator"
	}
	return "root"
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
