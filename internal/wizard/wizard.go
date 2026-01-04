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

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/postalsys/muti-metroo/internal/certutil"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/service"
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
	theme      *huh.Theme
	existingCfg *config.Config // Loaded from existing config file, if any
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

	// Step 11: Service installation (on supported platforms)
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

func (w *Wizard) askBasicSetup() (dataDir, configPath, displayName string, err error) {
	dataDir = "./data"
	configPath = "./config.yaml"
	displayName = ""

	// First, ask for config path so we can try to load existing config
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Basic Setup").
				Description("Configure the essential paths for your agent."),

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

	if err = pathForm.Run(); err != nil {
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
	form := huh.NewForm(
		huh.NewGroup(
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
				Title("Display Name").
				Description("Human-readable name for this agent (Unicode allowed, e.g., \"Tallinn Gateway\")").
				Placeholder("(press Enter to use Agent ID)").
				Value(&displayName),
		),
	).WithTheme(w.theme)

	err = form.Run()
	return
}

func (w *Wizard) askAgentRoles() ([]string, error) {
	var roles []string

	// Try to infer roles from existing config
	if w.existingCfg != nil {
		if w.existingCfg.SOCKS5.Enabled {
			roles = append(roles, "ingress")
		}
		if w.existingCfg.Exit.Enabled {
			roles = append(roles, "exit")
		}
		// If has peers but not ingress/exit, assume transit
		if len(w.existingCfg.Peers) > 0 && !w.existingCfg.SOCKS5.Enabled && !w.existingCfg.Exit.Enabled {
			roles = append(roles, "transit")
		}
		// Default to transit if nothing else
		if len(roles) == 0 {
			roles = append(roles, "transit")
		}
	}

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

	// Use existing config defaults if available
	if w.existingCfg != nil && len(w.existingCfg.Listeners) > 0 {
		l := w.existingCfg.Listeners[0]
		transport = l.Transport
		listenAddr = l.Address
		if l.Path != "" {
			path = l.Path
		}
	}

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

func (w *Wizard) askTLSSetup(dataDir string) (certsDir string, tlsConfig config.GlobalTLSConfig, err error) {
	certsDir = filepath.Join(dataDir, "certs")
	var tlsChoice string
	var enableMTLS bool = true // Default to mTLS enabled

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

			huh.NewConfirm().
				Title("Enable mTLS (mutual TLS)?").
				Description("Require client certificates for peer authentication").
				Value(&enableMTLS),
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

	// Set mTLS preference
	tlsConfig.MTLS = enableMTLS

	return
}

func (w *Wizard) generateCertificates(certsDir string) (config.GlobalTLSConfig, error) {
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
		return config.GlobalTLSConfig{}, err
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
				Description("Paste CA certificate for peer verification and mTLS").
				CharLimit(10000).
				Value(&caContent),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
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
				Description("For peer verification and mTLS").
				Placeholder(caPath).
				Value(&caPath),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
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

	// Use existing config defaults if available
	if w.existingCfg != nil && w.existingCfg.SOCKS5.Address != "" {
		cfg.Address = w.existingCfg.SOCKS5.Address
		cfg.MaxConnections = w.existingCfg.SOCKS5.MaxConnections
		enableAuth = w.existingCfg.SOCKS5.Auth.Enabled
	}

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

	// Use existing config defaults if available
	if w.existingCfg != nil && len(w.existingCfg.Exit.Routes) > 0 {
		routesStr = strings.Join(w.existingCfg.Exit.Routes, "\n")
		cfg.DNS = w.existingCfg.Exit.DNS
	}

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

func (w *Wizard) askAdvancedOptions() (healthEnabled bool, logLevel string, err error) {
	healthEnabled = true
	logLevel = "info"

	// Use existing config defaults if available
	if w.existingCfg != nil {
		healthEnabled = w.existingCfg.HTTP.Enabled
		logLevel = w.existingCfg.Agent.LogLevel
	}

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
				Description("HTTP endpoint for monitoring (/health, /healthz) and CLI commands").
				Value(&healthEnabled),
		),
	).WithTheme(w.theme)

	err = form.Run()
	return
}

func (w *Wizard) askShellConfig() (config.ShellConfig, error) {
	cfg := config.ShellConfig{
		Enabled:     false,
		Whitelist:   []string{},
		MaxSessions: 0, // 0 = unlimited
	}
	var enableShell bool

	// Use existing config defaults if available
	if w.existingCfg != nil {
		enableShell = w.existingCfg.Shell.Enabled
		cfg.Timeout = w.existingCfg.Shell.Timeout
		cfg.MaxSessions = w.existingCfg.Shell.MaxSessions
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Remote Shell Access").
				Description("Shell allows executing commands remotely on this agent.\nCommands must be whitelisted for security."),

			huh.NewConfirm().
				Title("Enable Remote Shell?").
				Description("Allow remote command execution (requires authentication)").
				Value(&enableShell),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return cfg, err
	}

	if !enableShell {
		return cfg, nil
	}

	cfg.Enabled = true

	// Ask for password
	var password, confirmPassword string
	authForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Shell Authentication").
				Description("Set a password to protect shell access.\nThis password will be hashed and stored securely."),

			huh.NewInput().
				Title("Shell Password").
				EchoMode(huh.EchoModePassword).
				Value(&password).
				Validate(func(s string) error {
					if len(s) < 8 {
						return fmt.Errorf("password must be at least 8 characters")
					}
					return nil
				}),

			huh.NewInput().
				Title("Confirm Password").
				EchoMode(huh.EchoModePassword).
				Value(&confirmPassword).
				Validate(func(s string) error {
					if s != password {
						return fmt.Errorf("passwords do not match")
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err := authForm.Run(); err != nil {
		return cfg, err
	}

	// Hash the password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return cfg, fmt.Errorf("failed to hash password: %w", err)
	}
	cfg.PasswordHash = string(hash)

	// Ask for whitelist
	var whitelistChoice string
	whitelistForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Command Whitelist").
				Description("Only whitelisted commands can be executed.\nFor security, the default is no commands allowed."),

			huh.NewSelect[string]().
				Title("Whitelist Mode").
				Options(
					huh.NewOption("No commands (safest)", "none"),
					huh.NewOption("Allow all commands (testing only!)", "all"),
					huh.NewOption("Custom whitelist", "custom"),
				).
				Value(&whitelistChoice),
		),
	).WithTheme(w.theme)

	if err := whitelistForm.Run(); err != nil {
		return cfg, err
	}

	switch whitelistChoice {
	case "none":
		cfg.Whitelist = []string{}
	case "all":
		cfg.Whitelist = []string{"*"}
		fmt.Print("\n[WARNING] All commands are allowed! Use only for testing.\n\n")
	case "custom":
		var commandsStr string
		customForm := huh.NewForm(
			huh.NewGroup(
				huh.NewText().
					Title("Allowed Commands").
					Description("Enter one command per line (e.g., whoami, ip, hostname, bash)").
					Placeholder("whoami\nhostname\nbash").
					Value(&commandsStr).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("at least one command is required")
						}
						return nil
					}),
			),
		).WithTheme(w.theme)

		if err := customForm.Run(); err != nil {
			return cfg, err
		}

		// Parse commands
		for _, line := range strings.Split(commandsStr, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				cfg.Whitelist = append(cfg.Whitelist, line)
			}
		}
	}

	return cfg, nil
}

func (w *Wizard) askFileTransferConfig() (config.FileTransferConfig, error) {
	cfg := config.FileTransferConfig{
		Enabled:      false,
		MaxFileSize:  500 * 1024 * 1024, // 500MB
		AllowedPaths: []string{},
	}
	var enableFileTransfer bool

	// Use existing config defaults if available
	if w.existingCfg != nil {
		enableFileTransfer = w.existingCfg.FileTransfer.Enabled
		if w.existingCfg.FileTransfer.MaxFileSize > 0 {
			cfg.MaxFileSize = w.existingCfg.FileTransfer.MaxFileSize
		}
		cfg.AllowedPaths = w.existingCfg.FileTransfer.AllowedPaths
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("File Transfer").
				Description("File transfer allows uploading and downloading files to/from this agent.\nFiles are transferred via the control channel."),

			huh.NewConfirm().
				Title("Enable file transfer?").
				Description("Allow remote file upload/download (requires authentication)").
				Value(&enableFileTransfer),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return cfg, err
	}

	if !enableFileTransfer {
		return cfg, nil
	}

	cfg.Enabled = true

	// Ask for password
	var password, confirmPassword string
	authForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("File Transfer Authentication").
				Description("Set a password to protect file transfer access.\nThis password will be hashed and stored securely."),

			huh.NewInput().
				Title("File Transfer Password").
				EchoMode(huh.EchoModePassword).
				Value(&password).
				Validate(func(s string) error {
					if len(s) < 8 {
						return fmt.Errorf("password must be at least 8 characters")
					}
					return nil
				}),

			huh.NewInput().
				Title("Confirm Password").
				EchoMode(huh.EchoModePassword).
				Value(&confirmPassword).
				Validate(func(s string) error {
					if s != password {
						return fmt.Errorf("passwords do not match")
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err := authForm.Run(); err != nil {
		return cfg, err
	}

	// Hash the password using bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return cfg, fmt.Errorf("failed to hash password: %w", err)
	}
	cfg.PasswordHash = string(hash)

	// Ask for max file size
	var maxSizeMB string = "500"
	sizeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Max File Size (MB)").
				Description("Maximum allowed file size in megabytes").
				Placeholder("500").
				Value(&maxSizeMB).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					size, err := strconv.Atoi(s)
					if err != nil || size < 1 {
						return fmt.Errorf("must be a positive number")
					}
					return nil
				}),
		),
	).WithTheme(w.theme)

	if err := sizeForm.Run(); err != nil {
		return cfg, err
	}

	if size, err := strconv.Atoi(maxSizeMB); err == nil && size > 0 {
		cfg.MaxFileSize = int64(size) * 1024 * 1024
	}

	// Ask for path restrictions
	var pathRestriction string
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Path Restrictions").
				Description("You can restrict file operations to specific directories.\nLeave empty to allow all absolute paths."),

			huh.NewSelect[string]().
				Title("Path Access").
				Options(
					huh.NewOption("Allow all paths (no restrictions)", "all"),
					huh.NewOption("Restrict to specific directories", "restricted"),
				).
				Value(&pathRestriction),
		),
	).WithTheme(w.theme)

	if err := pathForm.Run(); err != nil {
		return cfg, err
	}

	if pathRestriction == "restricted" {
		var pathsStr string
		customPathForm := huh.NewForm(
			huh.NewGroup(
				huh.NewText().
					Title("Allowed Paths").
					Description("Enter one path prefix per line (e.g., /tmp, /home/user/uploads)").
					Placeholder("/tmp\n/var/uploads").
					Value(&pathsStr).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("at least one path is required")
						}
						return nil
					}),
			),
		).WithTheme(w.theme)

		if err := customPathForm.Run(); err != nil {
			return cfg, err
		}

		// Parse paths
		for _, line := range strings.Split(pathsStr, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				cfg.AllowedPaths = append(cfg.AllowedPaths, line)
			}
		}
	}

	return cfg, nil
}

func (w *Wizard) askManagementKey() (config.ManagementConfig, error) {
	cfg := config.ManagementConfig{}
	var choice string

	// Use existing config defaults if available
	if w.existingCfg != nil && w.existingCfg.Management.PublicKey != "" {
		// If there's an existing management key, offer to keep it
		var keepExisting bool
		keepForm := huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("Management Key Encryption (OPSEC Protection)").
					Description("Existing management key configuration found."),

				huh.NewConfirm().
					Title("Keep existing management key?").
					Description("Current public key: " + w.existingCfg.Management.PublicKey[:16] + "...").
					Value(&keepExisting),
			),
		).WithTheme(w.theme)

		if err := keepForm.Run(); err != nil {
			return cfg, err
		}

		if keepExisting {
			return w.existingCfg.Management, nil
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Management Key Encryption (OPSEC Protection)").
				Description("Encrypt mesh topology data so only operators can view it.\nCompromised agents will only see encrypted blobs.\n\nThis is recommended for red team operations."),

			huh.NewSelect[string]().
				Title("Management Key Setup").
				Options(
					huh.NewOption("Skip (not recommended for red team ops)", "skip"),
					huh.NewOption("Generate new management keypair", "generate"),
					huh.NewOption("Enter existing public key", "existing"),
				).
				Value(&choice),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
		return cfg, err
	}

	switch choice {
	case "skip":
		return cfg, nil

	case "generate":
		keypair, err := identity.NewKeypair()
		if err != nil {
			return cfg, fmt.Errorf("failed to generate management keypair: %w", err)
		}

		cfg.PublicKey = hex.EncodeToString(keypair.PublicKey[:])

		// Ask if this is an operator node
		var isOperator bool
		operatorForm := huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("Operator Node").
					Description("Operator nodes can view mesh topology data.\nField agents should NOT have the private key."),

				huh.NewConfirm().
					Title("Is this an operator/management node?").
					Description("Only operator nodes should have the private key").
					Value(&isOperator),
			),
		).WithTheme(w.theme)

		if err := operatorForm.Run(); err != nil {
			return cfg, err
		}

		if isOperator {
			cfg.PrivateKey = hex.EncodeToString(keypair.PrivateKey[:])
		}

		// Always show the private key so operator can save it
		warningStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("208"))

		keyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

		fmt.Println()
		fmt.Println(warningStyle.Render("=== SAVE THIS MANAGEMENT KEYPAIR ==="))
		fmt.Println()
		fmt.Println("Public Key (add to ALL agent configs):")
		fmt.Println(keyStyle.Render("  " + hex.EncodeToString(keypair.PublicKey[:])))
		fmt.Println()
		fmt.Println("Private Key (add to OPERATOR config only, keep secure!):")
		fmt.Println(keyStyle.Render("  " + hex.EncodeToString(keypair.PrivateKey[:])))
		fmt.Println()

		if isOperator {
			fmt.Println("[INFO] Private key will be included in this config.")
		} else {
			fmt.Println("[INFO] Private key NOT included in this config (field agent).")
		}
		fmt.Println()

	case "existing":
		var pubKey string
		inputForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Management Public Key").
					Description("64-character hex string from operator").
					Placeholder("a1b2c3d4e5f6...").
					Value(&pubKey).
					Validate(func(s string) error {
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
					}),
			),
		).WithTheme(w.theme)

		if err := inputForm.Run(); err != nil {
			return cfg, err
		}

		// Normalize the key
		pubKey = strings.TrimSpace(pubKey)
		pubKey = strings.TrimPrefix(pubKey, "0x")
		pubKey = strings.TrimPrefix(pubKey, "0X")
		cfg.PublicKey = pubKey

		// Ask if this is an operator node with the private key
		var hasPrivateKey bool
		privKeyForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you have the private key?").
					Description("Only operator nodes should have the private key").
					Value(&hasPrivateKey),
			),
		).WithTheme(w.theme)

		if err := privKeyForm.Run(); err != nil {
			return cfg, err
		}

		if hasPrivateKey {
			var privKey string
			privInputForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Management Private Key").
						Description("64-character hex string").
						EchoMode(huh.EchoModePassword).
						Value(&privKey).
						Validate(func(s string) error {
							s = strings.TrimSpace(s)
							s = strings.TrimPrefix(s, "0x")
							s = strings.TrimPrefix(s, "0X")
							if len(s) != 64 {
								return fmt.Errorf("private key must be 64 hex characters (got %d)", len(s))
							}
							if _, err := hex.DecodeString(s); err != nil {
								return fmt.Errorf("invalid hex string: %v", err)
							}
							return nil
						}),
				),
			).WithTheme(w.theme)

			if err := privInputForm.Run(); err != nil {
				return cfg, err
			}

			// Normalize
			privKey = strings.TrimSpace(privKey)
			privKey = strings.TrimPrefix(privKey, "0x")
			privKey = strings.TrimPrefix(privKey, "0X")
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
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("42"))

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("─────────────────────────────────────────────────")

	fmt.Println()
	fmt.Println(divider)
	fmt.Println(style.Render("[OK] Setup Complete!"))
	fmt.Println(divider)
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
	var installService bool
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
		var showInstructions bool

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("Service Installation").
					Description(fmt.Sprintf("To install as a %s, elevated privileges are required.\nYou can run the service install command later with %s.", platformName, privilegeCmd)),

				huh.NewConfirm().
					Title("Show installation command?").
					Description("Display the command to install the service").
					Value(&showInstructions),
			),
		).WithTheme(w.theme)

		if err := form.Run(); err != nil {
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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Service Installation").
				Description(fmt.Sprintf("You are running as %s.\nWould you like to install Muti Metroo as a %s?\n\nThe service will start automatically on boot.", w.privilegeLevel(), platformName)),

			huh.NewConfirm().
				Title(fmt.Sprintf("Install as %s?", platformName)).
				Description("The agent will run in the background").
				Value(&installService),
		),
	).WithTheme(w.theme)

	if err := form.Run(); err != nil {
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
	successStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("42"))
	fmt.Println(successStyle.Render(fmt.Sprintf("[OK] Installed as %s", platformName)))

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
