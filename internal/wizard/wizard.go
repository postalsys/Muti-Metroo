// Package wizard provides an interactive setup wizard for Muti Metroo.
package wizard

import (
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
	"github.com/postalsys/muti-metroo/internal/identity"
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
}

// Wizard manages the interactive setup process.
type Wizard struct {
	existingCfg *config.Config // Loaded from existing config file, if any
}

// New creates a new setup wizard.
func New() *Wizard {
	return &Wizard{}
}

// Run executes the interactive setup wizard.
func (w *Wizard) Run() (*Result, error) {
	w.printBanner()

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
	transport, listenAddr, listenPath, err := w.askNetworkConfig()
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

	// Build configuration
	cfg := w.buildConfig(
		dataDir, displayName, transport, listenAddr, listenPath,
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

	// Write configuration file
	if err := w.writeConfig(cfg, configPath); err != nil {
		return nil, err
	}

	// Print summary
	w.printSummary(agentID, keypair, configPath, cfg)

	// Step 12: Service installation (on supported platforms)
	var serviceInstalled bool
	if service.IsSupported() {
		serviceInstalled, err = w.askServiceInstallation(configPath)
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
	}, nil
}

func (w *Wizard) printBanner() {
	prompt.PrintBanner("Muti Metroo Setup Wizard", "Userspace Mesh Networking Agent")
	fmt.Println("By using this software, you agree to the Terms of Service:")
	fmt.Println("https://mutimetroo.com/terms-of-service")
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

func (w *Wizard) askNetworkConfig() (transport, listenAddr, path string, err error) {
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
	}

	prompt.PrintHeader("Network Configuration", "Configure how this agent listens for connections.")

	// Transport selection
	transportOptions := []string{
		"QUIC (UDP, fastest)",
		"HTTP/2 (TCP, firewall-friendly)",
		"WebSocket (TCP, proxy-friendly)",
	}
	transportMap := []string{"quic", "h2", "ws"}

	defaultIdx := 0
	for i, t := range transportMap {
		if t == transport {
			defaultIdx = i
			break
		}
	}

	fmt.Println("Transport Protocol (QUIC is recommended for best performance):")
	idx, err := prompt.Select("Select", transportOptions, defaultIdx)
	if err != nil {
		return
	}
	transport = transportMap[idx]

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

	return
}

func (w *Wizard) askTLSSetup(dataDir string) (certsDir string, tlsConfig config.GlobalTLSConfig, err error) {
	certsDir = filepath.Join(dataDir, "certs")

	prompt.PrintHeader("TLS Configuration", "TLS is required for secure communication.\nYou can generate new certificates or use existing ones.")

	// Certificate setup choice
	tlsOptions := []string{
		"Generate new self-signed certificates (Recommended for testing)",
		"Paste certificate and key content",
		"Use existing certificate files",
	}

	idx, err := prompt.Select("Certificate Setup", tlsOptions, 0)
	if err != nil {
		return
	}

	certsDir, err = prompt.ReadLine("Certificates Directory", certsDir)
	if err != nil {
		return
	}

	enableMTLS, err := prompt.Confirm("Enable mTLS (mutual TLS)?", true)
	if err != nil {
		return
	}

	// Ensure certs directory exists
	if err = os.MkdirAll(certsDir, 0700); err != nil {
		return certsDir, tlsConfig, fmt.Errorf("failed to create certs directory: %w", err)
	}

	switch idx {
	case 0: // generate
		tlsConfig, err = w.generateCertificates(certsDir)
	case 1: // paste
		tlsConfig, err = w.pasteCertificates(certsDir)
	case 2: // existing
		tlsConfig, err = w.useExistingCertificates(certsDir)
	}

	// Set mTLS preference
	tlsConfig.MTLS = enableMTLS

	return
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
		peers = append(peers, peer)

		addMore, err = prompt.Confirm("Add another peer?", false)
		if err != nil {
			return nil, err
		}
	}

	return peers, nil
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
	transportOptions := []string{"QUIC", "HTTP/2", "WebSocket"}
	transportMap := []string{"quic", "h2", "ws"}

	defaultIdx := 0
	for i, t := range transportMap {
		if t == defaultTransport {
			defaultIdx = i
			break
		}
	}

	idx, err := prompt.Select("Transport", transportOptions, defaultIdx)
	if err != nil {
		return peer, err
	}
	peer.Transport = transportMap[idx]

	useInsecure, err := prompt.Confirm("Skip TLS verification? (only for testing with self-signed certs)", false)
	if err != nil {
		return peer, err
	}
	peer.TLS.InsecureSkipVerify = useInsecure

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
	return cfg, nil
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

	var password string
	for {
		password, err = prompt.ReadPassword("Shell Password (min 8 chars)")
		if err != nil {
			return cfg, err
		}
		if len(password) < 8 {
			fmt.Println("  Error: password must be at least 8 characters")
			continue
		}

		confirmPassword, err := prompt.ReadPassword("Confirm Password")
		if err != nil {
			return cfg, err
		}
		if password != confirmPassword {
			fmt.Println("  Error: passwords do not match")
			continue
		}
		break
	}

	// Hash the password
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

	var password string
	for {
		password, err = prompt.ReadPassword("File Transfer Password (min 8 chars)")
		if err != nil {
			return cfg, err
		}
		if len(password) < 8 {
			fmt.Println("  Error: password must be at least 8 characters")
			continue
		}

		confirmPassword, err := prompt.ReadPassword("Confirm Password")
		if err != nil {
			return cfg, err
		}
		if password != confirmPassword {
			fmt.Println("  Error: passwords do not match")
			continue
		}
		break
	}

	// Hash the password using bcrypt
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
			s = strings.TrimSpace(s)
			s = strings.TrimPrefix(s, "0x")
			s = strings.TrimPrefix(s, "0X")
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

		// Normalize the key
		pubKey = strings.TrimSpace(pubKey)
		pubKey = strings.TrimPrefix(pubKey, "0x")
		pubKey = strings.TrimPrefix(pubKey, "0X")
		cfg.PublicKey = pubKey

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

			privKey = strings.TrimSpace(privKey)
			privKey = strings.TrimPrefix(privKey, "0x")
			privKey = strings.TrimPrefix(privKey, "0X")

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

func (w *Wizard) askServiceInstallation(configPath string) (bool, error) {
	var platformName string
	var privilegeCmd string

	switch runtime.GOOS {
	case "linux":
		platformName = "systemd service"
		privilegeCmd = "sudo"
	case "darwin":
		platformName = "launchd service"
		privilegeCmd = "sudo"
	case "windows":
		platformName = "Windows service"
		privilegeCmd = "Run as Administrator"
	default:
		return false, nil
	}

	// Get absolute path for config
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute config path: %w", err)
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
			switch runtime.GOOS {
			case "linux", "darwin":
				fmt.Printf("  sudo muti-metroo service install -c %s\n", absConfigPath)
			case "windows":
				fmt.Printf("  muti-metroo service install -c %s\n", absConfigPath)
				fmt.Println("  (Run this command as Administrator)")
			}
			fmt.Println()
		}

		return false, nil
	}

	prompt.PrintHeader("Service Installation",
		fmt.Sprintf("You are running as %s.\nWould you like to install Muti Metroo as a %s?\n\nThe service will start automatically on boot.", w.privilegeLevel(), platformName))

	installService, err := prompt.Confirm(fmt.Sprintf("Install as %s?", platformName), false)
	if err != nil {
		return false, err
	}

	if !installService {
		return false, nil
	}

	cfg := service.DefaultConfig(absConfigPath)

	fmt.Println()
	fmt.Printf("Installing %s...\n", platformName)

	if err := service.Install(cfg); err != nil {
		// Don't fail the wizard, just warn
		fmt.Printf("\n[WARNING] Failed to install service: %v\n", err)
		fmt.Println("You can install the service manually later.")
		return false, nil
	}

	fmt.Println()
	fmt.Printf("[OK] Installed as %s\n", platformName)

	switch runtime.GOOS {
	case "linux":
		fmt.Println("\nUseful commands:")
		fmt.Println("  systemctl status muti-metroo    # Check status")
		fmt.Println("  journalctl -u muti-metroo -f    # View logs")
		fmt.Println("  systemctl restart muti-metroo   # Restart service")
		fmt.Println("  muti-metroo service uninstall   # Remove service")
	case "darwin":
		fmt.Println("\nUseful commands:")
		fmt.Println("  sudo launchctl list | grep muti   # Check if running")
		fmt.Println("  tail -f /var/log/muti-metroo.log  # View logs")
		fmt.Println("  sudo launchctl stop com.muti-metroo   # Stop service")
		fmt.Println("  sudo launchctl start com.muti-metroo  # Start service")
		fmt.Println("  muti-metroo service uninstall   # Remove service")
	case "windows":
		fmt.Println("\nUseful commands:")
		fmt.Println("  sc query muti-metroo            # Check status")
		fmt.Println("  net start muti-metroo           # Start service")
		fmt.Println("  net stop muti-metroo            # Stop service")
		fmt.Println("  muti-metroo service uninstall   # Remove service")
	}
	fmt.Println()

	return true, nil
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
