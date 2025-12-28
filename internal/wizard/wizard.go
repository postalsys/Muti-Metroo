// Package wizard provides an interactive setup wizard for Muti Metroo.
package wizard

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/coinstash/muti-metroo/internal/certutil"
	"github.com/coinstash/muti-metroo/internal/config"
	"github.com/coinstash/muti-metroo/internal/identity"
	"gopkg.in/yaml.v3"
)

// Result contains the wizard output.
type Result struct {
	Config     *config.Config
	ConfigPath string
	DataDir    string
	CertsDir   string
}

// Wizard manages the interactive setup process.
type Wizard struct {
	theme *huh.Theme
}

// New creates a new setup wizard.
func New() *Wizard {
	return &Wizard{
		theme: huh.ThemeDracula(),
	}
}

// Run executes the interactive setup wizard.
func (w *Wizard) Run() (*Result, error) {
	w.printBanner()

	// Step 1: Basic setup
	dataDir, configPath, err := w.askBasicSetup()
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
	healthEnabled, controlEnabled, logLevel, err := w.askAdvancedOptions()
	if err != nil {
		return nil, err
	}

	// Build configuration
	cfg := w.buildConfig(
		dataDir, transport, listenAddr, listenPath,
		tlsConfig, peers, socks5Config, exitConfig,
		healthEnabled, controlEnabled, logLevel,
	)

	// Initialize identity
	agentID, _, err := identity.LoadOrCreate(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize agent identity: %w", err)
	}

	// Write configuration file
	if err := w.writeConfig(cfg, configPath); err != nil {
		return nil, err
	}

	// Print summary
	w.printSummary(agentID, configPath, cfg)

	return &Result{
		Config:     cfg,
		ConfigPath: configPath,
		DataDir:    dataDir,
		CertsDir:   certsDir,
	}, nil
}

func (w *Wizard) printBanner() {
	banner := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")).
		Render(`
  __  __       _   _   __  __      _
 |  \/  |_   _| |_(_) |  \/  | ___| |_ _ __ ___   ___
 | |\/| | | | | __| | | |\/| |/ _ \ __| '__/ _ \ / _ \
 | |  | | |_| | |_| | | |  | |  __/ |_| | | (_) | (_) |
 |_|  |_|\__,_|\__|_| |_|  |_|\___|\__|_|  \___/ \___/
`)

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("  Userspace Mesh Networking Agent - Setup Wizard\n")

	fmt.Println(banner)
	fmt.Println(subtitle)
}

func (w *Wizard) askBasicSetup() (dataDir, configPath string, err error) {
	dataDir = "./data"
	configPath = "./config.yaml"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Basic Setup").
				Description("Configure the essential paths for your agent."),

			huh.NewInput().
				Title("Data Directory").
				Description("Where to store agent identity and state").
				Placeholder("./data").
				Value(&dataDir).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("data directory is required")
					}
					return nil
				}),

			huh.NewInput().
				Title("Config File Path").
				Description("Where to write the configuration file").
				Placeholder("./config.yaml").
				Value(&configPath).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("config path is required")
					}
					if !strings.HasSuffix(s, ".yaml") && !strings.HasSuffix(s, ".yml") {
						return fmt.Errorf("config file should have .yaml or .yml extension")
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	err = form.Run()
	return
}

func (w *Wizard) askAgentRoles() ([]string, error) {
	var roles []string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Agent Role").
				Description("Select the roles this agent will perform.\nYou can select multiple roles."),

			huh.NewMultiSelect[string]().
				Title("Select Roles").
				Options(
					huh.NewOption("Ingress (SOCKS5 proxy entry point)", "ingress"),
					huh.NewOption("Transit (relay traffic between peers)", "transit"),
					huh.NewOption("Exit (connect to external networks)", "exit"),
				).
				Value(&roles).
				Validate(func(s []string) error {
					if len(s) == 0 {
						return fmt.Errorf("select at least one role")
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return nil, err
	}

	return roles, nil
}

func (w *Wizard) askNetworkConfig() (transport, listenAddr, path string, err error) {
	transport = "quic"
	listenAddr = "0.0.0.0:4433"
	path = "/mesh"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Network Configuration").
				Description("Configure how this agent listens for connections."),

			huh.NewSelect[string]().
				Title("Transport Protocol").
				Description("QUIC is recommended for best performance").
				Options(
					huh.NewOption("QUIC (UDP, fastest)", "quic"),
					huh.NewOption("HTTP/2 (TCP, firewall-friendly)", "h2"),
					huh.NewOption("WebSocket (TCP, proxy-friendly)", "ws"),
				).
				Value(&transport),

			huh.NewInput().
				Title("Listen Address").
				Description("Address and port to listen on").
				Placeholder("0.0.0.0:4433").
				Value(&listenAddr).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("listen address is required")
					}
					_, _, err := net.SplitHostPort(s)
					if err != nil {
						return fmt.Errorf("invalid address format (use host:port)")
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err = form.Run(); err != nil {
		return
	}

	// Ask for path if using HTTP-based transport
	if transport == "h2" || transport == "ws" {
		pathForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("HTTP Path").
					Description("URL path for the HTTP endpoint").
					Placeholder("/mesh").
					Value(&path).
					Validate(func(s string) error {
						if s == "" || !strings.HasPrefix(s, "/") {
							return fmt.Errorf("path must start with /")
						}
						return nil
					}),
			),
		).WithTheme(w.theme)

		if err = pathForm.Run(); err != nil {
			return
		}
	}

	return
}

func (w *Wizard) askTLSSetup(dataDir string) (certsDir string, tlsConfig config.TLSConfig, err error) {
	certsDir = filepath.Join(dataDir, "certs")
	var tlsChoice string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("TLS Configuration").
				Description("TLS is required for secure communication.\nYou can generate new certificates or use existing ones."),

			huh.NewSelect[string]().
				Title("Certificate Setup").
				Options(
					huh.NewOption("Generate new self-signed certificates (Recommended for testing)", "generate"),
					huh.NewOption("Paste certificate and key content", "paste"),
					huh.NewOption("Use existing certificate files", "existing"),
				).
				Value(&tlsChoice),

			huh.NewInput().
				Title("Certificates Directory").
				Description("Where to store/find certificate files").
				Placeholder(certsDir).
				Value(&certsDir),
		),
	).WithTheme(w.theme)

	if err = form.Run(); err != nil {
		return
	}

	// Ensure certs directory exists
	if err = os.MkdirAll(certsDir, 0700); err != nil {
		return certsDir, tlsConfig, fmt.Errorf("failed to create certs directory: %w", err)
	}

	switch tlsChoice {
	case "generate":
		tlsConfig, err = w.generateCertificates(certsDir)
	case "paste":
		tlsConfig, err = w.pasteCertificates(certsDir)
	case "existing":
		tlsConfig, err = w.useExistingCertificates(certsDir)
	}

	return
}

func (w *Wizard) generateCertificates(certsDir string) (config.TLSConfig, error) {
	var commonName string = "muti-metroo"
	var validDays int = 365

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Generate Certificates").
				Description("A CA and server certificate will be generated."),

			huh.NewInput().
				Title("Common Name").
				Description("Name for the certificate (e.g., hostname)").
				Placeholder("muti-metroo").
				Value(&commonName),

			huh.NewInput().
				Title("Validity (days)").
				Description("How long the certificate should be valid").
				Placeholder("365").
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					d, err := strconv.Atoi(s)
					if err != nil || d < 1 {
						return fmt.Errorf("must be a positive number")
					}
					validDays = d
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return config.TLSConfig{}, err
	}

	// Generate CA
	validFor := time.Duration(validDays) * 24 * time.Hour
	ca, err := certutil.GenerateCA(commonName+" CA", validFor)
	if err != nil {
		return config.TLSConfig{}, fmt.Errorf("failed to generate CA: %w", err)
	}

	caPath := filepath.Join(certsDir, "ca.crt")
	caKeyPath := filepath.Join(certsDir, "ca.key")
	if err := ca.SaveToFiles(caPath, caKeyPath); err != nil {
		return config.TLSConfig{}, fmt.Errorf("failed to save CA: %w", err)
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
		return config.TLSConfig{}, fmt.Errorf("failed to generate certificate: %w", err)
	}

	certPath := filepath.Join(certsDir, "server.crt")
	keyPath := filepath.Join(certsDir, "server.key")
	if err := cert.SaveToFiles(certPath, keyPath); err != nil {
		return config.TLSConfig{}, fmt.Errorf("failed to save certificate: %w", err)
	}

	fmt.Printf("\n✓ Generated CA certificate: %s\n", caPath)
	fmt.Printf("✓ Generated server certificate: %s\n", certPath)
	fmt.Printf("  Fingerprint: %s\n\n", cert.Fingerprint())

	return config.TLSConfig{
		Cert:     certPath,
		Key:      keyPath,
		CA:       caPath,
		ClientCA: caPath,
	}, nil
}

func (w *Wizard) pasteCertificates(certsDir string) (config.TLSConfig, error) {
	var certContent, keyContent, caContent string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Paste Certificate").
				Description("Paste your PEM-encoded certificate content.\nInclude the BEGIN/END markers."),

			huh.NewText().
				Title("Certificate (PEM)").
				Description("Paste server certificate").
				CharLimit(10000).
				Value(&certContent).
				Validate(func(s string) error {
					if !strings.Contains(s, "-----BEGIN CERTIFICATE-----") {
						return fmt.Errorf("invalid certificate format")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewText().
				Title("Private Key (PEM)").
				Description("Paste private key").
				CharLimit(10000).
				Value(&keyContent).
				Validate(func(s string) error {
					if !strings.Contains(s, "-----BEGIN") || !strings.Contains(s, "PRIVATE KEY-----") {
						return fmt.Errorf("invalid private key format")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewText().
				Title("CA Certificate (PEM) - Optional").
				Description("Paste CA certificate for client verification").
				CharLimit(10000).
				Value(&caContent),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return config.TLSConfig{}, err
	}

	// Write certificate files
	certPath := filepath.Join(certsDir, "server.crt")
	keyPath := filepath.Join(certsDir, "server.key")

	if err := os.WriteFile(certPath, []byte(certContent), 0644); err != nil {
		return config.TLSConfig{}, fmt.Errorf("failed to write certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(keyContent), 0600); err != nil {
		return config.TLSConfig{}, fmt.Errorf("failed to write key: %w", err)
	}

	tlsConfig := config.TLSConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	if caContent != "" && strings.Contains(caContent, "-----BEGIN CERTIFICATE-----") {
		caPath := filepath.Join(certsDir, "ca.crt")
		if err := os.WriteFile(caPath, []byte(caContent), 0644); err != nil {
			return config.TLSConfig{}, fmt.Errorf("failed to write CA: %w", err)
		}
		tlsConfig.CA = caPath
		tlsConfig.ClientCA = caPath
	}

	fmt.Printf("\n✓ Saved certificate to: %s\n", certPath)
	fmt.Printf("✓ Saved private key to: %s\n\n", keyPath)

	return tlsConfig, nil
}

func (w *Wizard) useExistingCertificates(certsDir string) (config.TLSConfig, error) {
	certPath := filepath.Join(certsDir, "server.crt")
	keyPath := filepath.Join(certsDir, "server.key")
	caPath := filepath.Join(certsDir, "ca.crt")

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Existing Certificates").
				Description("Specify paths to your existing certificate files."),

			huh.NewInput().
				Title("Certificate File").
				Placeholder(certPath).
				Value(&certPath).
				Validate(func(s string) error {
					if _, err := os.Stat(s); os.IsNotExist(err) {
						return fmt.Errorf("file not found: %s", s)
					}
					return nil
				}),

			huh.NewInput().
				Title("Private Key File").
				Placeholder(keyPath).
				Value(&keyPath).
				Validate(func(s string) error {
					if _, err := os.Stat(s); os.IsNotExist(err) {
						return fmt.Errorf("file not found: %s", s)
					}
					return nil
				}),

			huh.NewInput().
				Title("CA Certificate File (optional)").
				Placeholder(caPath).
				Value(&caPath),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return config.TLSConfig{}, err
	}

	tlsConfig := config.TLSConfig{
		Cert: certPath,
		Key:  keyPath,
	}

	if caPath != "" {
		if _, err := os.Stat(caPath); err == nil {
			tlsConfig.CA = caPath
			tlsConfig.ClientCA = caPath
		}
	}

	return tlsConfig, nil
}

func (w *Wizard) askPeerConnections(transport string) ([]config.PeerConfig, error) {
	var addPeers bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Peer Connections").
				Description("Configure connections to other mesh agents."),

			huh.NewConfirm().
				Title("Add peer connections?").
				Description("Connect to other agents in the mesh").
				Value(&addPeers),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
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

		confirmForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Add another peer?").
					Value(&addMore),
			),
		).WithTheme(w.theme)

		if err := confirmForm.Run(); err != nil {
			return nil, err
		}
	}

	return peers, nil
}

func (w *Wizard) askSinglePeer(defaultTransport string, peerNum int) (config.PeerConfig, error) {
	peer := config.PeerConfig{
		Transport: defaultTransport,
	}
	var peerAddr, peerPath, peerID string
	var useInsecure bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Peer #%d", peerNum)),

			huh.NewInput().
				Title("Peer Address").
				Description("Address of the peer (host:port)").
				Placeholder("peer.example.com:4433").
				Value(&peerAddr).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("address is required")
					}
					_, _, err := net.SplitHostPort(s)
					if err != nil {
						return fmt.Errorf("invalid address format")
					}
					return nil
				}),

			huh.NewInput().
				Title("Expected Agent ID").
				Description("The agent ID you expect to connect to (hex string)").
				Placeholder("auto").
				Value(&peerID),

			huh.NewSelect[string]().
				Title("Transport").
				Options(
					huh.NewOption("QUIC", "quic"),
					huh.NewOption("HTTP/2", "h2"),
					huh.NewOption("WebSocket", "ws"),
				).
				Value(&peer.Transport),

			huh.NewConfirm().
				Title("Skip TLS verification?").
				Description("Only use for testing with self-signed certs").
				Value(&useInsecure),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return peer, err
	}

	peer.Address = peerAddr
	if peerID == "" || peerID == "auto" {
		peer.ID = "auto"
	} else {
		peer.ID = peerID
	}

	peer.TLS.InsecureSkipVerify = useInsecure

	// Ask for path if HTTP transport
	if peer.Transport == "h2" || peer.Transport == "ws" {
		peerPath = "/mesh"
		pathForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("HTTP Path").
					Placeholder("/mesh").
					Value(&peerPath),
			),
		).WithTheme(w.theme)

		if err := pathForm.Run(); err != nil {
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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("SOCKS5 Proxy").
				Description("Configure the SOCKS5 ingress proxy."),

			huh.NewInput().
				Title("Listen Address").
				Description("Address for SOCKS5 proxy").
				Placeholder("127.0.0.1:1080").
				Value(&cfg.Address).
				Validate(func(s string) error {
					_, _, err := net.SplitHostPort(s)
					return err
				}),

			huh.NewConfirm().
				Title("Enable authentication?").
				Description("Require username/password for SOCKS5").
				Value(&enableAuth),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return cfg, err
	}

	if enableAuth {
		var username, password string
		authForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Username").
					Value(&username).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("username required")
						}
						return nil
					}),
				huh.NewInput().
					Title("Password").
					EchoMode(huh.EchoModePassword).
					Value(&password).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("password required")
						}
						return nil
					}),
			),
		).WithTheme(w.theme)

		if err := authForm.Run(); err != nil {
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
	var routesStr string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Exit Node Configuration").
				Description("Configure this agent as an exit node.\nIt will allow traffic to specified networks."),

			huh.NewText().
				Title("Allowed Routes (CIDR)").
				Description("One CIDR per line (e.g., 0.0.0.0/0 for all traffic)").
				Placeholder("0.0.0.0/0\n::/0").
				Value(&routesStr).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("at least one route is required")
					}
					for _, line := range strings.Split(s, "\n") {
						line = strings.TrimSpace(line)
						if line == "" {
							continue
						}
						if _, _, err := net.ParseCIDR(line); err != nil {
							return fmt.Errorf("invalid CIDR: %s", line)
						}
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return cfg, err
	}

	// Parse routes
	for _, line := range strings.Split(routesStr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			cfg.Routes = append(cfg.Routes, line)
		}
	}

	return cfg, nil
}

func (w *Wizard) askAdvancedOptions() (healthEnabled, controlEnabled bool, logLevel string, err error) {
	healthEnabled = true
	controlEnabled = true
	logLevel = "info"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Advanced Options").
				Description("Configure monitoring and logging."),

			huh.NewSelect[string]().
				Title("Log Level").
				Options(
					huh.NewOption("Debug (verbose)", "debug"),
					huh.NewOption("Info (recommended)", "info"),
					huh.NewOption("Warning", "warn"),
					huh.NewOption("Error (quiet)", "error"),
				).
				Value(&logLevel),

			huh.NewConfirm().
				Title("Enable health check endpoint?").
				Description("HTTP endpoint for monitoring (/health, /healthz)").
				Value(&healthEnabled),

			huh.NewConfirm().
				Title("Enable control socket?").
				Description("Unix socket for CLI commands (status, peers, routes)").
				Value(&controlEnabled),
		),
	).WithTheme(w.theme)

	err = form.Run()
	return
}

func (w *Wizard) buildConfig(
	dataDir, transport, listenAddr, listenPath string,
	tlsConfig config.TLSConfig,
	peers []config.PeerConfig,
	socks5Config config.SOCKS5Config,
	exitConfig config.ExitConfig,
	healthEnabled, controlEnabled bool,
	logLevel string,
) *config.Config {
	cfg := config.Default()

	cfg.Agent.DataDir = dataDir
	cfg.Agent.LogLevel = logLevel
	cfg.Agent.LogFormat = "text"

	// Listener
	listener := config.ListenerConfig{
		Transport: transport,
		Address:   listenAddr,
		TLS:       tlsConfig,
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

	// Health
	cfg.Health.Enabled = healthEnabled
	if healthEnabled {
		cfg.Health.Address = ":8080"
	}

	// Control
	cfg.Control.Enabled = controlEnabled
	if controlEnabled {
		cfg.Control.SocketPath = filepath.Join(dataDir, "control.sock")
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
# See https://github.com/coinstash/muti-metroo for documentation

`
	if err := os.WriteFile(path, []byte(header+string(data)), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (w *Wizard) printSummary(agentID identity.AgentID, configPath string, cfg *config.Config) {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("42"))

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("─────────────────────────────────────────────────")

	fmt.Println()
	fmt.Println(divider)
	fmt.Println(style.Render("✓ Setup Complete!"))
	fmt.Println(divider)
	fmt.Println()

	fmt.Printf("  Agent ID:     %s\n", agentID.String())
	fmt.Printf("  Config file:  %s\n", configPath)
	fmt.Printf("  Data dir:     %s\n", cfg.Agent.DataDir)
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

	if cfg.Health.Enabled {
		fmt.Printf("  Health:       http://%s/health\n", cfg.Health.Address)
	}

	fmt.Println()
	fmt.Println("  To start the agent:")
	fmt.Printf("    muti-metroo run -c %s\n", configPath)
	fmt.Println()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
