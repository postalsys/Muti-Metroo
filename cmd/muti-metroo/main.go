// Package main provides the CLI entry point for Muti Metroo mesh agent.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/postalsys/muti-metroo/internal/agent"
	"github.com/postalsys/muti-metroo/internal/certutil"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/filetransfer"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/licenses"
	"github.com/postalsys/muti-metroo/internal/service"
	"github.com/postalsys/muti-metroo/internal/shell"
	"github.com/postalsys/muti-metroo/internal/wizard"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var (
	// Version is set at build time
	Version = "dev"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "muti-metroo",
		Short: "Muti Metroo - Userspace mesh networking agent",
		Long: `Muti Metroo is a userspace mesh networking agent that creates
virtual TCP tunnels across heterogeneous transport layers.

It enables multi-hop routing with SOCKS5 ingress and CIDR-based
exit routing, operating entirely in userspace without requiring
root privileges.`,
		Version: Version,
	}

	// Define command groups for organized help output
	rootCmd.AddGroup(&cobra.Group{ID: "start", Title: "Getting Started:"})
	rootCmd.AddGroup(&cobra.Group{ID: "status", Title: "Agent Status:"})
	rootCmd.AddGroup(&cobra.Group{ID: "remote", Title: "Remote Operations:"})
	rootCmd.AddGroup(&cobra.Group{ID: "admin", Title: "Administration:"})

	// Getting Started commands
	setup := setupCmd()
	setup.GroupID = "start"
	rootCmd.AddCommand(setup)

	initC := initCmd()
	initC.GroupID = "start"
	rootCmd.AddCommand(initC)

	run := runCmd()
	run.GroupID = "start"
	rootCmd.AddCommand(run)

	// Agent Status commands
	status := statusCmd()
	status.GroupID = "status"
	rootCmd.AddCommand(status)

	peers := peersCmd()
	peers.GroupID = "status"
	rootCmd.AddCommand(peers)

	routes := routesCmd()
	routes.GroupID = "status"
	rootCmd.AddCommand(routes)

	// Remote Operations commands
	shellC := shellCmd()
	shellC.GroupID = "remote"
	rootCmd.AddCommand(shellC)

	upload := uploadCmd()
	upload.GroupID = "remote"
	rootCmd.AddCommand(upload)

	download := downloadCmd()
	download.GroupID = "remote"
	rootCmd.AddCommand(download)

	// Administration commands
	svc := serviceCmd()
	svc.GroupID = "admin"
	rootCmd.AddCommand(svc)

	cert := certCmd()
	cert.GroupID = "admin"
	rootCmd.AddCommand(cert)

	hash := hashCmd()
	hash.GroupID = "admin"
	rootCmd.AddCommand(hash)

	mgmtKey := managementKeyCmd()
	mgmtKey.GroupID = "admin"
	rootCmd.AddCommand(mgmtKey)

	lic := licensesCmd()
	lic.GroupID = "admin"
	rootCmd.AddCommand(lic)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func setupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard",
		Long: `Run an interactive setup wizard to configure the mesh agent.

The wizard will guide you through:
  - Basic configuration (data directory, config file path)
  - Agent role selection (ingress, transit, exit)
  - Network configuration (transport, listen address)
  - TLS setup (generate, paste, or use existing certificates)
  - Peer connections
  - SOCKS5 proxy settings (for ingress nodes)
  - Exit node configuration (for exit nodes)
  - Advanced options (logging, health checks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := wizard.New()
			result, err := w.Run()
			if err != nil {
				return fmt.Errorf("setup wizard failed: %w", err)
			}

			_ = result // Result contains the generated config
			return nil
		},
	}

	return cmd
}

func initCmd() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new agent",
		Long:  "Initialize a new agent by creating data directory and generating identity.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if already initialized
			if identity.Exists(dataDir) {
				id, err := identity.Load(dataDir)
				if err != nil {
					return fmt.Errorf("failed to load existing identity: %w", err)
				}
				fmt.Printf("Agent already initialized in %s\n", dataDir)
				fmt.Printf("Agent ID: %s\n", id.String())
				return nil
			}

			// Create new identity
			id, created, err := identity.LoadOrCreate(dataDir)
			if err != nil {
				return fmt.Errorf("failed to initialize agent: %w", err)
			}

			if created {
				fmt.Printf("Agent initialized in %s\n", dataDir)
				fmt.Printf("Agent ID: %s\n", id.String())
			} else {
				fmt.Printf("Agent already exists in %s\n", dataDir)
				fmt.Printf("Agent ID: %s\n", id.String())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&dataDir, "data-dir", "d", "./data", "Directory for persistent state")

	return cmd
}

func runCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the mesh agent",
		Long:  "Start the mesh agent with the specified configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Create agent
			a, err := agent.New(cfg)
			if err != nil {
				return fmt.Errorf("failed to create agent: %w", err)
			}

			// Check if running as Windows service
			if !service.IsInteractive() {
				// Running as Windows service - use service handler
				return service.RunAsService("muti-metroo", a)
			}

			// Running interactively (console mode)
			fmt.Printf("Starting Muti Metroo agent...\n")
			if cfg.Agent.DisplayName != "" {
				fmt.Printf("Display Name: %s\n", cfg.Agent.DisplayName)
			}
			fmt.Printf("Agent ID: %s\n", a.ID().String())

			// Start agent
			if err := a.Start(); err != nil {
				return fmt.Errorf("failed to start agent: %w", err)
			}

			// Print status
			stats := a.Stats()
			if cfg.SOCKS5.Enabled {
				fmt.Printf("SOCKS5 server: %s\n", cfg.SOCKS5.Address)
			}
			if cfg.Exit.Enabled {
				fmt.Printf("Exit routes: %v\n", cfg.Exit.Routes)
			}
			fmt.Printf("Status: running (peers: %d, routes: %d)\n", stats.PeerCount, stats.RouteCount)

			// Wait for shutdown signal
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			sig := <-sigCh
			fmt.Printf("\nReceived signal %v, shutting down...\n", sig)

			// Graceful shutdown with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := a.StopWithContext(ctx); err != nil {
				fmt.Printf("Shutdown error: %v\n", err)
				return err
			}

			fmt.Println("Agent stopped.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "./config.yaml", "Path to configuration file")

	return cmd
}

func statusCmd() *cobra.Command {
	var agentAddr string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agent status",
		Long: `Display the current status of the running agent via HTTP API.

Shows running agent information including:
  - Agent status (OK/error)
  - Connected peer count
  - Active stream count
  - Route table size
  - SOCKS5 proxy status
  - Exit handler status

Example output:
  Agent Status
  ============
  Status:       OK
  Running:      true
  Peer Count:   3
  Stream Count: 12
  Route Count:  5
  SOCKS5:       true
  Exit Handler: false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			url := fmt.Sprintf("http://%s/healthz", agentAddr)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to connect to agent: %w", err)
			}
			defer resp.Body.Close()

			var status struct {
				Status            string `json:"status"`
				Running           bool   `json:"running"`
				PeerCount         int    `json:"peer_count"`
				StreamCount       int    `json:"stream_count"`
				RouteCount        int    `json:"route_count"`
				SOCKS5Running     bool   `json:"socks5_running"`
				ExitHandlerRunning bool  `json:"exit_handler_running"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}

			fmt.Printf("Agent Status\n")
			fmt.Printf("============\n")
			fmt.Printf("Status:       %s\n", status.Status)
			fmt.Printf("Running:      %v\n", status.Running)
			fmt.Printf("Peer Count:   %d\n", status.PeerCount)
			fmt.Printf("Stream Count: %d\n", status.StreamCount)
			fmt.Printf("Route Count:  %d\n", status.RouteCount)
			fmt.Printf("SOCKS5:       %v\n", status.SOCKS5Running)
			fmt.Printf("Exit Handler: %v\n", status.ExitHandlerRunning)

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Agent API address (host:port)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func peersCmd() *cobra.Command {
	var agentAddr string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "peers",
		Short: "List connected peers",
		Long:  "Display all peers currently connected to this agent via HTTP API.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			url := fmt.Sprintf("http://%s/api/dashboard", agentAddr)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to connect to agent: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status: %d", resp.StatusCode)
			}

			var dashboard struct {
				Peers []struct {
					ID          string `json:"id"`
					ShortID     string `json:"short_id"`
					DisplayName string `json:"display_name"`
					State       string `json:"state"`
					RTTMs       int64  `json:"rtt_ms"`
					IsDialer    bool   `json:"is_dialer"`
				} `json:"peers"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&dashboard); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(dashboard.Peers)
			}

			fmt.Printf("Connected Peers\n")
			fmt.Printf("===============\n")
			if len(dashboard.Peers) == 0 {
				fmt.Println("No peers connected.")
			} else {
				fmt.Printf("%-12s %-20s %-10s %-10s %-8s\n", "ID", "NAME", "STATE", "ROLE", "RTT")
				fmt.Printf("%-12s %-20s %-10s %-10s %-8s\n", "--", "----", "-----", "----", "---")
				for _, peer := range dashboard.Peers {
					role := "listener"
					if peer.IsDialer {
						role = "dialer"
					}
					rtt := fmt.Sprintf("%dms", peer.RTTMs)
					if peer.RTTMs == 0 {
						rtt = "-"
					}
					fmt.Printf("%-12s %-20s %-10s %-10s %-8s\n",
						peer.ShortID,
						peer.DisplayName,
						peer.State,
						role,
						rtt,
					)
				}
				fmt.Printf("\nTotal: %d peer(s)\n", len(dashboard.Peers))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Agent API address (host:port)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func routesCmd() *cobra.Command {
	var agentAddr string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "routes",
		Short: "List route table",
		Long:  "Display the current routing table via HTTP API.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			url := fmt.Sprintf("http://%s/api/dashboard", agentAddr)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to connect to agent: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status: %d", resp.StatusCode)
			}

			var dashboard struct {
				Routes []struct {
					Network     string   `json:"network"`
					NextHopID   string   `json:"next_hop_id"`
					NextHopName string   `json:"next_hop_name"`
					OriginID    string   `json:"origin_id"`
					OriginName  string   `json:"origin_name"`
					Metric      int      `json:"metric"`
					HopCount    int      `json:"hop_count"`
					PathDisplay []string `json:"path_display"`
				} `json:"routes"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&dashboard); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(dashboard.Routes)
			}

			fmt.Printf("Route Table\n")
			fmt.Printf("===========\n")
			if len(dashboard.Routes) == 0 {
				fmt.Println("No routes in table.")
			} else {
				fmt.Printf("%-20s %-15s %-15s %-8s %-6s\n", "NETWORK", "NEXT HOP", "ORIGIN", "METRIC", "HOPS")
				fmt.Printf("%-20s %-15s %-15s %-8s %-6s\n", "-------", "--------", "------", "------", "----")
				for _, route := range dashboard.Routes {
					nextHop := route.NextHopName
					if nextHop == "" {
						nextHop = route.NextHopID
					}
					origin := route.OriginName
					if origin == "" {
						origin = route.OriginID
					}
					fmt.Printf("%-20s %-15s %-15s %-8d %-6d\n",
						route.Network,
						nextHop,
						origin,
						route.Metric,
						route.HopCount,
					)
				}
				fmt.Printf("\nTotal: %d route(s)\n", len(dashboard.Routes))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Agent API address (host:port)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func serviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "System service management",
		Long: `Manage Muti Metroo as a system service.

Supported platforms:
  - Linux: systemd
  - macOS: launchd
  - Windows: Windows Service`,
	}

	cmd.AddCommand(serviceInstallCmd())
	cmd.AddCommand(serviceUninstallCmd())
	cmd.AddCommand(serviceStatusCmd())

	return cmd
}

func serviceInstallCmd() *cobra.Command {
	var configPath string
	var serviceName string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install as a system service",
		Long: `Install Muti Metroo as a system service.

On Linux, this creates and enables a systemd service.
On macOS, this creates and loads a launchd service.
On Windows, this registers a Windows service.

This command requires root/administrator privileges.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check platform support
			if !service.IsSupported() {
				return fmt.Errorf("service management is not supported on %s", runtime.GOOS)
			}

			// Check privileges
			if !service.IsRoot() {
				switch runtime.GOOS {
				case "linux", "darwin":
					return fmt.Errorf("must run as root to install the service (try: sudo muti-metroo service install ...)")
				case "windows":
					return fmt.Errorf("must run as Administrator to install the service")
				}
			}

			// Validate config file exists
			if configPath == "" {
				return fmt.Errorf("config file is required: use -c flag")
			}

			absPath, err := filepath.Abs(configPath)
			if err != nil {
				return fmt.Errorf("failed to resolve config path: %w", err)
			}

			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				return fmt.Errorf("config file not found: %s", absPath)
			}

			// Check if already installed
			if service.IsInstalled(serviceName) {
				return fmt.Errorf("service '%s' is already installed", serviceName)
			}

			// Create service config
			cfg := service.DefaultConfig(absPath)
			cfg.Name = serviceName

			// Install
			fmt.Printf("Installing service '%s'...\n", serviceName)
			fmt.Printf("  Config: %s\n", absPath)
			fmt.Printf("  Platform: %s\n", service.Platform())

			if err := service.Install(cfg); err != nil {
				return fmt.Errorf("failed to install service: %w", err)
			}

			fmt.Println("\nService installed successfully.")

			switch runtime.GOOS {
			case "linux":
				fmt.Println("\nManage the service with:")
				fmt.Println("  sudo systemctl status muti-metroo")
				fmt.Println("  sudo systemctl restart muti-metroo")
				fmt.Println("  sudo journalctl -u muti-metroo -f")
			case "darwin":
				fmt.Println("\nManage the service with:")
				fmt.Println("  sudo launchctl list com.muti-metroo")
				fmt.Printf("  tail -f %s/%s.log\n", cfg.WorkingDir, serviceName)
			case "windows":
				fmt.Println("\nManage the service with:")
				fmt.Println("  sc query muti-metroo")
				fmt.Println("  sc stop muti-metroo")
				fmt.Println("  sc start muti-metroo")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file (required)")
	cmd.Flags().StringVarP(&serviceName, "name", "n", "muti-metroo", "Service name")
	cmd.MarkFlagRequired("config")

	return cmd
}

func serviceUninstallCmd() *cobra.Command {
	var serviceName string
	var force bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the system service",
		Long: `Remove the Muti Metroo system service.

On Linux, this stops and removes the systemd service.
On macOS, this unloads and removes the launchd service.
On Windows, this stops and removes the Windows service.

This command requires root/administrator privileges.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check platform support
			if !service.IsSupported() {
				return fmt.Errorf("service management is not supported on %s", runtime.GOOS)
			}

			// Check privileges
			if !service.IsRoot() {
				switch runtime.GOOS {
				case "linux", "darwin":
					return fmt.Errorf("must run as root to uninstall the service (try: sudo muti-metroo service uninstall)")
				case "windows":
					return fmt.Errorf("must run as Administrator to uninstall the service")
				}
			}

			// Check if installed
			if !service.IsInstalled(serviceName) {
				fmt.Printf("Service '%s' is not installed.\n", serviceName)
				return nil
			}

			// Confirm unless force flag is set
			if !force {
				fmt.Printf("This will stop and remove the '%s' service.\n", serviceName)
				fmt.Print("Continue? [y/N]: ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" && response != "yes" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			// Uninstall
			if err := service.Uninstall(serviceName); err != nil {
				return fmt.Errorf("failed to uninstall service: %w", err)
			}

			fmt.Println("\nService uninstalled successfully.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&serviceName, "name", "n", "muti-metroo", "Service name")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func serviceStatusCmd() *cobra.Command {
	var serviceName string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show service status",
		Long:  `Show the current status of the Muti Metroo system service.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check platform support
			if !service.IsSupported() {
				return fmt.Errorf("service management is not supported on %s", runtime.GOOS)
			}

			// Check if installed
			if !service.IsInstalled(serviceName) {
				fmt.Printf("Service '%s' is not installed.\n", serviceName)
				return nil
			}

			// Get status
			status, err := service.Status(serviceName)
			if err != nil {
				return fmt.Errorf("failed to get service status: %w", err)
			}

			fmt.Printf("Service: %s\n", serviceName)
			fmt.Printf("Status: %s\n", status)
			fmt.Printf("Platform: %s\n", service.Platform())

			return nil
		},
	}

	cmd.Flags().StringVarP(&serviceName, "name", "n", "muti-metroo", "Service name")

	return cmd
}

func certCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "Certificate management commands",
		Long:  "Generate and manage TLS certificates for the mesh network.",
	}

	cmd.AddCommand(certCACmd())
	cmd.AddCommand(certAgentCmd())
	cmd.AddCommand(certClientCmd())
	cmd.AddCommand(certInfoCmd())

	return cmd
}

func certCACmd() *cobra.Command {
	var (
		commonName string
		outDir     string
		validDays  int
	)

	cmd := &cobra.Command{
		Use:   "ca",
		Short: "Generate a CA certificate",
		Long:  "Generate a new Certificate Authority certificate and private key.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commonName == "" {
				commonName = "Muti Metroo CA"
			}

			validFor := time.Duration(validDays) * 24 * time.Hour

			fmt.Printf("Generating CA certificate...\n")
			fmt.Printf("  Common Name: %s\n", commonName)
			fmt.Printf("  Valid for: %d days\n", validDays)

			ca, err := certutil.GenerateCA(commonName, validFor)
			if err != nil {
				return fmt.Errorf("failed to generate CA: %w", err)
			}

			certPath := outDir + "/ca.crt"
			keyPath := outDir + "/ca.key"

			if err := ca.SaveToFiles(certPath, keyPath); err != nil {
				return fmt.Errorf("failed to save CA: %w", err)
			}

			fmt.Printf("\nCA certificate generated:\n")
			fmt.Printf("  Certificate: %s\n", certPath)
			fmt.Printf("  Private key: %s\n", keyPath)
			fmt.Printf("  Fingerprint: %s\n", ca.Fingerprint())
			fmt.Printf("  Expires: %s\n", ca.Certificate.NotAfter.Format(time.RFC3339))

			return nil
		},
	}

	cmd.Flags().StringVar(&commonName, "cn", "Muti Metroo CA", "Common name for the CA")
	cmd.Flags().StringVarP(&outDir, "out", "o", "./certs", "Output directory for certificate files")
	cmd.Flags().IntVar(&validDays, "days", 365, "Validity period in days")

	return cmd
}

func certAgentCmd() *cobra.Command {
	var (
		commonName string
		outDir     string
		validDays  int
		caPath     string
		caKeyPath  string
		dnsNames   string
		ipAddrs    string
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Generate an agent/peer certificate",
		Long:  "Generate a new agent certificate signed by a CA. The certificate can be used for both server and client authentication.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commonName == "" {
				return fmt.Errorf("common name is required")
			}

			// Load CA
			ca, err := certutil.LoadCert(caPath, caKeyPath)
			if err != nil {
				return fmt.Errorf("failed to load CA: %w", err)
			}

			validFor := time.Duration(validDays) * 24 * time.Hour

			fmt.Printf("Generating agent certificate...\n")
			fmt.Printf("  Common Name: %s\n", commonName)
			fmt.Printf("  Valid for: %d days\n", validDays)
			fmt.Printf("  CA: %s\n", ca.Certificate.Subject.CommonName)

			// Build options
			opts := certutil.DefaultPeerOptions(commonName)
			opts.ValidFor = validFor
			opts.ParentCert = ca.Certificate
			opts.ParentKey = ca.PrivateKey

			// Add DNS names
			if dnsNames != "" {
				opts.DNSNames = append(opts.DNSNames, strings.Split(dnsNames, ",")...)
			}

			// Add IP addresses
			if ipAddrs != "" {
				for _, ip := range strings.Split(ipAddrs, ",") {
					parsed := net.ParseIP(strings.TrimSpace(ip))
					if parsed == nil {
						return fmt.Errorf("invalid IP address: %s", ip)
					}
					opts.IPAddresses = append(opts.IPAddresses, parsed)
				}
			}

			cert, err := certutil.GenerateCert(opts)
			if err != nil {
				return fmt.Errorf("failed to generate certificate: %w", err)
			}

			certPath := outDir + "/" + commonName + ".crt"
			keyPath := outDir + "/" + commonName + ".key"

			if err := cert.SaveToFiles(certPath, keyPath); err != nil {
				return fmt.Errorf("failed to save certificate: %w", err)
			}

			fmt.Printf("\nAgent certificate generated:\n")
			fmt.Printf("  Certificate: %s\n", certPath)
			fmt.Printf("  Private key: %s\n", keyPath)
			fmt.Printf("  Fingerprint: %s\n", cert.Fingerprint())
			fmt.Printf("  Expires: %s\n", cert.Certificate.NotAfter.Format(time.RFC3339))
			if len(opts.DNSNames) > 0 {
				fmt.Printf("  DNS Names: %s\n", strings.Join(opts.DNSNames, ", "))
			}
			if len(opts.IPAddresses) > 0 {
				ips := make([]string, len(opts.IPAddresses))
				for i, ip := range opts.IPAddresses {
					ips[i] = ip.String()
				}
				fmt.Printf("  IP Addresses: %s\n", strings.Join(ips, ", "))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&commonName, "cn", "", "Common name for the certificate (required)")
	cmd.Flags().StringVarP(&outDir, "out", "o", "./certs", "Output directory for certificate files")
	cmd.Flags().IntVar(&validDays, "days", 90, "Validity period in days")
	cmd.Flags().StringVar(&caPath, "ca", "./certs/ca.crt", "Path to CA certificate")
	cmd.Flags().StringVar(&caKeyPath, "ca-key", "./certs/ca.key", "Path to CA private key")
	cmd.Flags().StringVar(&dnsNames, "dns", "", "Additional DNS names (comma-separated)")
	cmd.Flags().StringVar(&ipAddrs, "ip", "", "Additional IP addresses (comma-separated)")

	_ = cmd.MarkFlagRequired("cn")

	return cmd
}

func certClientCmd() *cobra.Command {
	var (
		commonName string
		outDir     string
		validDays  int
		caPath     string
		caKeyPath  string
	)

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Generate a client certificate",
		Long:  "Generate a new client certificate signed by a CA. The certificate is for client authentication only.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commonName == "" {
				return fmt.Errorf("common name is required")
			}

			// Load CA
			ca, err := certutil.LoadCert(caPath, caKeyPath)
			if err != nil {
				return fmt.Errorf("failed to load CA: %w", err)
			}

			validFor := time.Duration(validDays) * 24 * time.Hour

			fmt.Printf("Generating client certificate...\n")
			fmt.Printf("  Common Name: %s\n", commonName)
			fmt.Printf("  Valid for: %d days\n", validDays)
			fmt.Printf("  CA: %s\n", ca.Certificate.Subject.CommonName)

			cert, err := certutil.GenerateClientCert(commonName, validFor, ca)
			if err != nil {
				return fmt.Errorf("failed to generate certificate: %w", err)
			}

			certPath := outDir + "/" + commonName + ".crt"
			keyPath := outDir + "/" + commonName + ".key"

			if err := cert.SaveToFiles(certPath, keyPath); err != nil {
				return fmt.Errorf("failed to save certificate: %w", err)
			}

			fmt.Printf("\nClient certificate generated:\n")
			fmt.Printf("  Certificate: %s\n", certPath)
			fmt.Printf("  Private key: %s\n", keyPath)
			fmt.Printf("  Fingerprint: %s\n", cert.Fingerprint())
			fmt.Printf("  Expires: %s\n", cert.Certificate.NotAfter.Format(time.RFC3339))

			return nil
		},
	}

	cmd.Flags().StringVar(&commonName, "cn", "", "Common name for the certificate (required)")
	cmd.Flags().StringVarP(&outDir, "out", "o", "./certs", "Output directory for certificate files")
	cmd.Flags().IntVar(&validDays, "days", 90, "Validity period in days")
	cmd.Flags().StringVar(&caPath, "ca", "./certs/ca.crt", "Path to CA certificate")
	cmd.Flags().StringVar(&caKeyPath, "ca-key", "./certs/ca.key", "Path to CA private key")

	_ = cmd.MarkFlagRequired("cn")

	return cmd
}

func certInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <certificate>",
		Short: "Display certificate information",
		Long:  "Display detailed information about a certificate file.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			certPath := args[0]

			info, err := certutil.GetCertInfoFromFile(certPath)
			if err != nil {
				return fmt.Errorf("failed to read certificate: %w", err)
			}

			fmt.Printf("Certificate: %s\n\n", certPath)
			fmt.Printf("Subject:      %s\n", info.Subject)
			fmt.Printf("Issuer:       %s\n", info.Issuer)
			fmt.Printf("Serial:       %s\n", info.SerialNumber)
			fmt.Printf("Fingerprint:  %s\n", info.Fingerprint)
			fmt.Printf("Is CA:        %v\n", info.IsCA)
			fmt.Printf("Not Before:   %s\n", info.NotBefore.Format(time.RFC3339))
			fmt.Printf("Not After:    %s\n", info.NotAfter.Format(time.RFC3339))

			// Check expiration
			now := time.Now()
			if now.After(info.NotAfter) {
				fmt.Printf("Status:       EXPIRED\n")
			} else if now.Add(30 * 24 * time.Hour).After(info.NotAfter) {
				daysLeft := int(info.NotAfter.Sub(now).Hours() / 24)
				fmt.Printf("Status:       EXPIRING SOON (%d days left)\n", daysLeft)
			} else {
				daysLeft := int(info.NotAfter.Sub(now).Hours() / 24)
				fmt.Printf("Status:       Valid (%d days left)\n", daysLeft)
			}

			if len(info.DNSNames) > 0 {
				fmt.Printf("DNS Names:    %s\n", strings.Join(info.DNSNames, ", "))
			}
			if len(info.IPAddresses) > 0 {
				fmt.Printf("IP Addresses: %s\n", strings.Join(info.IPAddresses, ", "))
			}
			if len(info.KeyUsage) > 0 {
				fmt.Printf("Key Usage:    %s\n", strings.Join(info.KeyUsage, ", "))
			}
			if len(info.ExtKeyUsage) > 0 {
				fmt.Printf("Ext Key Usage: %s\n", strings.Join(info.ExtKeyUsage, ", "))
			}

			return nil
		},
	}

	return cmd
}

func shellCmd() *cobra.Command {
	var (
		agentAddr  string
		password   string
		timeoutStr string
		ttyMode    bool
	)

	cmd := &cobra.Command{
		Use:   "shell [flags] <target-agent-id> [command] [args...]",
		Short: "Run commands on a remote agent",
		Long: `Run commands on a remote agent via shell.

The <target-agent-id> is the final destination agent where the command executes.
The --agent flag specifies which gateway agent to connect through (defaults to localhost).

By default, shell runs in streaming mode without a PTY, suitable for simple
commands like 'whoami', 'ls', or long-running output like 'tail -f'.
Use --tty for interactive mode when you need a full terminal (vim, htop, bash).

Streaming mode (default):
  - No PTY allocation
  - Separate stdout/stderr streams
  - Suitable for simple commands and continuous output

Interactive mode (--tty):
  - Allocates a PTY on the remote agent
  - Supports terminal resize (SIGWINCH)
  - Required for interactive programs (vim, less, htop, etc.)

Examples:
  # Simple command (streaming mode)
  muti-metroo shell abc123def456 whoami

  # Long-running output (streaming mode)
  muti-metroo shell abc123def456 tail -f /var/log/syslog

  # Follow logs (streaming mode)
  muti-metroo shell abc123def456 journalctl -u muti-metroo -f

  # Interactive bash shell (requires --tty)
  muti-metroo shell --tty abc123def456 bash

  # Interactive vim (requires --tty)
  muti-metroo shell --tty abc123def456 vim /etc/config.yaml

  # With password authentication
  muti-metroo shell -p secret abc123def456 whoami

  # Via a different agent
  muti-metroo shell -a 192.168.1.10:8080 abc123def456 top`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID := args[0]
			var command string
			var cmdArgs []string

			if len(args) > 1 {
				command = args[1]
				cmdArgs = args[2:]
			} else {
				// Default to shell if no command specified
				command = "bash"
			}

			// Parse timeout (supports duration strings like "5m" or plain seconds)
			timeoutSec, err := parseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}

			// Resolve short agent ID prefix to full ID
			resolvedID, err := resolveAgentID(targetID, agentAddr)
			if err != nil {
				return err
			}

			// Validate target agent ID
			if _, err := identity.ParseAgentID(resolvedID); err != nil {
				return fmt.Errorf("invalid agent ID '%s': %w", resolvedID, err)
			}

			// Create shell client
			client := shell.NewClient(shell.ClientConfig{
				AgentAddr:   agentAddr,
				TargetID:    resolvedID,
				Interactive: ttyMode,
				Password:    password,
				Command:     command,
				Args:        cmdArgs,
				Timeout:     timeoutSec,
			})

			// Run the shell session
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle interrupt signals
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			exitCode, err := client.Run(ctx)
			if err != nil {
				return err
			}

			if exitCode != 0 {
				os.Exit(exitCode)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Gateway agent API address (host:port)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Shell password for authentication")
	cmd.Flags().StringVarP(&timeoutStr, "timeout", "t", "0", "Session timeout (e.g., 30s, 5m, or 0 for no timeout)")
	cmd.Flags().BoolVar(&ttyMode, "tty", false, "Interactive mode with PTY (for vim, bash, htop, etc.)")

	return cmd
}

func uploadCmd() *cobra.Command {
	var (
		agentAddr  string
		password   string
		timeoutStr string
		rateLimit  string
		resume     bool
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "upload [flags] <target-agent-id> <local-path> <remote-path>",
		Short: "Upload a file or directory to a remote agent",
		Long: `Upload a local file or directory to a remote agent via the file transfer interface.

The <target-agent-id> is the final destination agent where the file is stored.
The --agent flag specifies which gateway agent to connect through (defaults to localhost).

File permissions (mode) are preserved. The remote path must be absolute.
Directories are automatically detected and uploaded as tar archives.

Examples:
  # Upload a file to a remote agent
  muti-metroo upload abc123def456 ./local/file.txt /tmp/remote-file.txt

  # Upload a large file (streaming, supports any size)
  muti-metroo upload abc123def456 ./large-iso.iso /tmp/large-iso.iso

  # Upload a directory (auto-detected)
  muti-metroo upload abc123def456 ./my-folder /tmp/my-folder

  # Via a different agent
  muti-metroo upload -a 192.168.1.10:8080 abc123def456 config.yaml /etc/app/config.yaml

  # With password authentication
  muti-metroo upload -p secret abc123def456 ./data.bin /home/user/data.bin

  # Rate-limited upload (100 KB/s)
  muti-metroo upload --rate-limit 100KB abc123def456 ./large.iso /tmp/large.iso

  # Resume an interrupted upload
  muti-metroo upload --resume abc123def456 ./huge.iso /tmp/huge.iso`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID := args[0]
			localPath := args[1]
			remotePath := args[2]

			// Parse timeout (supports duration strings like "5m" or plain seconds)
			timeoutSec, err := parseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}

			// Resolve short agent ID prefix to full ID
			resolvedID, err := resolveAgentID(targetID, agentAddr)
			if err != nil {
				return err
			}

			// Validate target agent ID
			if _, err := identity.ParseAgentID(resolvedID); err != nil {
				return fmt.Errorf("invalid agent ID '%s': %w", resolvedID, err)
			}

			// Validate remote path is absolute
			if !filepath.IsAbs(remotePath) {
				return fmt.Errorf("remote path must be absolute: %s", remotePath)
			}

			// Resolve local path
			absLocalPath, err := filepath.Abs(localPath)
			if err != nil {
				return fmt.Errorf("failed to resolve local path: %w", err)
			}

			// Check if local file/directory exists
			info, err := os.Stat(absLocalPath)
			if err != nil {
				return fmt.Errorf("cannot access local path: %w", err)
			}

			// Parse rate limit
			var rateLimitBytes int64
			if rateLimit != "" {
				rateLimitBytes, err = filetransfer.ParseSize(rateLimit)
				if err != nil {
					return fmt.Errorf("invalid rate limit: %w", err)
				}
			}

			// Resume not supported for directories
			if resume && info.IsDir() {
				fmt.Println("Warning: Resume not supported for directory uploads, starting fresh")
				resume = false
			}

			isDirectory := info.IsDir()
			return uploadFile(agentAddr, resolvedID, absLocalPath, remotePath, password, timeoutSec, isDirectory, rateLimitBytes, resume, quiet)
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Gateway agent API address (host:port)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "File transfer password for authentication")
	cmd.Flags().StringVarP(&timeoutStr, "timeout", "t", "5m", "Transfer timeout (e.g., 30s, 5m, 1h)")
	cmd.Flags().StringVar(&rateLimit, "rate-limit", "", "Maximum transfer speed (e.g., 100KB, 1MB, 10MiB)")
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume interrupted transfer if possible")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress progress output")

	return cmd
}

// uploadFile uploads a file or directory via multipart form streaming.
func uploadFile(agentAddr, targetID, localPath, remotePath, password string, timeout int, isDirectory bool, rateLimit int64, resume bool, quiet bool) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot access local path: %w", err)
	}

	// Create a pipe for the multipart form
	pr, pw := io.Pipe()

	// Create multipart writer
	writer := multipart.NewWriter(pw)

	// Progress tracking
	var totalSize int64
	if !isDirectory {
		totalSize = info.Size()
	}
	startTime := time.Now()
	var bytesWritten int64

	// Start goroutine to write form data
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		defer writer.Close()

		// Add form fields
		writer.WriteField("path", remotePath)
		if password != "" {
			writer.WriteField("password", password)
		}
		if isDirectory {
			writer.WriteField("directory", "true")
		}
		if rateLimit > 0 {
			writer.WriteField("rate_limit", fmt.Sprintf("%d", rateLimit))
		}
		if resume {
			writer.WriteField("resume", "true")
			// Also include original file size for validation
			writer.WriteField("original_size", fmt.Sprintf("%d", info.Size()))
		}

		// Create file part
		part, err := writer.CreateFormFile("file", filepath.Base(localPath))
		if err != nil {
			errCh <- fmt.Errorf("failed to create form file: %w", err)
			return
		}

		if isDirectory {
			// Tar and stream directory (no progress for directories)
			if !quiet {
				fmt.Printf("Uploading directory %s to %s:%s\n", localPath, targetID[:12], remotePath)
			}
			if err := filetransfer.TarDirectory(localPath, part); err != nil {
				errCh <- fmt.Errorf("failed to tar directory: %w", err)
				return
			}
		} else {
			// Stream file with progress tracking
			if !quiet {
				fmt.Printf("Uploading %s (%s) to %s:%s\n",
					filepath.Base(localPath), humanize.Bytes(uint64(info.Size())), targetID[:12], remotePath)
			}
			f, err := os.Open(localPath)
			if err != nil {
				errCh <- fmt.Errorf("failed to open file: %w", err)
				return
			}
			defer f.Close()

			// Create progress-tracking reader
			progressReader := &progressTrackingReader{
				reader:    f,
				total:     totalSize,
				written:   &bytesWritten,
				startTime: startTime,
				quiet:     quiet,
			}

			if _, err := io.Copy(part, progressReader); err != nil {
				errCh <- fmt.Errorf("failed to stream file: %w", err)
				return
			}
		}
		errCh <- nil
	}()

	// Build URL
	url := fmt.Sprintf("http://%s/agents/%s/file/upload", agentAddr, targetID)

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if quiet {
		// No progress output in quiet mode
	} else if isDirectory {
		fmt.Print("Uploading... ")
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)

	// Clear progress line if we were showing progress
	if !quiet && !isDirectory && totalSize > 0 {
		fmt.Print("\r" + strings.Repeat(" ", 70) + "\r") // Clear line
	}

	if err != nil {
		if !quiet {
			fmt.Println("FAILED")
		}
		// Check if there was an error in the goroutine
		if writeErr := <-errCh; writeErr != nil {
			return fmt.Errorf("upload error: %w (form write: %v)", err, writeErr)
		}
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Wait for goroutine
	if writeErr := <-errCh; writeErr != nil {
		if !quiet {
			fmt.Println("FAILED")
		}
		return writeErr
	}

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if !quiet {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var uploadResp struct {
		Success      bool   `json:"success"`
		Error        string `json:"error,omitempty"`
		BytesWritten int64  `json:"bytes_written"`
		RemotePath   string `json:"remote_path"`
	}
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		if !quiet {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("failed to parse response: %w (body: %s)", err, string(respBody))
	}

	if !uploadResp.Success {
		if !quiet {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("upload failed: %s", uploadResp.Error)
	}

	if !quiet {
		elapsed := time.Since(startTime)
		speed := float64(uploadResp.BytesWritten) / elapsed.Seconds()
		fmt.Printf("Uploaded %s to %s in %s (%s/s)\n",
			humanize.Bytes(uint64(uploadResp.BytesWritten)),
			remotePath,
			elapsed.Round(time.Millisecond),
			humanize.Bytes(uint64(speed)))
	}

	return nil
}

func downloadCmd() *cobra.Command {
	var (
		agentAddr  string
		password   string
		timeoutStr string
		rateLimit  string
		resume     bool
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "download [flags] <target-agent-id> <remote-path> <local-path>",
		Short: "Download a file or directory from a remote agent",
		Long: `Download a file or directory from a remote agent via the file transfer interface.

The <target-agent-id> is the final destination agent where the file is stored.
The --agent flag specifies which gateway agent to connect through (defaults to localhost).

File permissions (mode) are preserved. The remote path must be absolute.
Directories are automatically detected and downloaded as tar archives.

Examples:
  # Download a file from a remote agent
  muti-metroo download abc123def456 /tmp/remote-file.txt ./local/file.txt

  # Download a large file (streaming, supports any size)
  muti-metroo download abc123def456 /var/backup/large.iso ./large.iso

  # Download a directory (auto-detected)
  muti-metroo download abc123def456 /etc/myapp ./myapp-config

  # Via a different agent
  muti-metroo download -a 192.168.1.10:8080 abc123def456 /etc/app/config.yaml config.yaml

  # With password authentication
  muti-metroo download -p secret abc123def456 /home/user/data.bin ./data.bin

  # Rate-limited download (1 MB/s)
  muti-metroo download --rate-limit 1MB abc123def456 /data/backup.tar.gz ./backup.tar.gz

  # Resume an interrupted download
  muti-metroo download --resume abc123def456 /data/large.iso ./large.iso`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID := args[0]
			remotePath := args[1]
			localPath := args[2]

			// Parse timeout (supports duration strings like "5m" or plain seconds)
			timeoutSec, err := parseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}

			// Resolve short agent ID prefix to full ID
			resolvedID, err := resolveAgentID(targetID, agentAddr)
			if err != nil {
				return err
			}

			// Validate target agent ID
			if _, err := identity.ParseAgentID(resolvedID); err != nil {
				return fmt.Errorf("invalid agent ID '%s': %w", resolvedID, err)
			}

			// Validate remote path is absolute
			if !filepath.IsAbs(remotePath) {
				return fmt.Errorf("remote path must be absolute: %s", remotePath)
			}

			// Resolve local path
			absLocalPath, err := filepath.Abs(localPath)
			if err != nil {
				return fmt.Errorf("failed to resolve local path: %w", err)
			}

			// Parse rate limit
			var rateLimitBytes int64
			if rateLimit != "" {
				rateLimitBytes, err = filetransfer.ParseSize(rateLimit)
				if err != nil {
					return fmt.Errorf("invalid rate limit: %w", err)
				}
			}

			return downloadFile(agentAddr, resolvedID, remotePath, absLocalPath, password, timeoutSec, rateLimitBytes, resume, quiet)
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Gateway agent API address (host:port)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "File transfer password for authentication")
	cmd.Flags().StringVarP(&timeoutStr, "timeout", "t", "5m", "Transfer timeout (e.g., 30s, 5m, 1h)")
	cmd.Flags().StringVar(&rateLimit, "rate-limit", "", "Maximum transfer speed (e.g., 100KB, 1MB, 10MiB)")
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume interrupted transfer if possible")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress progress output")

	return cmd
}

// downloadFile downloads a file or directory via streaming.
func downloadFile(agentAddr, targetID, remotePath, localPath, password string, timeout int, rateLimit int64, resume bool, quiet bool) error {
	if !quiet {
		fmt.Printf("Downloading %s:%s to %s\n", targetID[:12], remotePath, localPath)
	}

	// Check for existing partial file if resume is requested
	var offset int64
	var originalSize int64
	if resume {
		partialInfo, err := filetransfer.HasPartialFile(localPath)
		if err != nil {
			if !quiet {
				fmt.Printf("Warning: failed to check partial file: %v\n", err)
			}
		} else if partialInfo != nil {
			offset = partialInfo.BytesWritten
			originalSize = partialInfo.OriginalSize
			if !quiet {
				fmt.Printf("Resuming from offset %s (of %s)\n",
					humanize.Bytes(uint64(offset)), humanize.Bytes(uint64(originalSize)))
			}
		}
	}

	// Build request
	reqBody := map[string]interface{}{
		"path": remotePath,
	}
	if password != "" {
		reqBody["password"] = password
	}
	if rateLimit > 0 {
		reqBody["rate_limit"] = rateLimit
	}
	if offset > 0 {
		reqBody["offset"] = offset
		reqBody["original_size"] = originalSize
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	// Build URL
	url := fmt.Sprintf("http://%s/agents/%s/file/download", agentAddr, targetID)

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if !quiet {
		if offset > 0 {
			fmt.Print("Resuming... ")
		} else {
			fmt.Print("Downloading... ")
		}
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		if !quiet {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for error response (JSON)
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			if !quiet {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("failed to read response: %w", err)
		}
		var errResp struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && !errResp.Success {
			if !quiet {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("download failed: %s", errResp.Error)
		}
		if !quiet {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("unexpected JSON response: %s", string(respBody))
	}

	// Check if it's a tar.gz (directory download)
	isTarGz := strings.HasPrefix(contentType, "application/gzip") ||
		strings.HasSuffix(resp.Header.Get("Content-Disposition"), ".tar.gz\"")

	if isTarGz {
		// Directories don't support resume
		if offset > 0 {
			if !quiet {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("resume not supported for directory downloads")
		}

		// Extract tar.gz to directory
		if err := os.MkdirAll(localPath, 0755); err != nil {
			if !quiet {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("failed to create directory: %w", err)
		}

		startTime := time.Now()
		if err := filetransfer.UntarDirectory(resp.Body, localPath); err != nil {
			if !quiet {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("failed to extract directory: %w", err)
		}

		elapsed := time.Since(startTime)
		if !quiet {
			fmt.Println("OK")
			fmt.Printf("Extracted directory to %s in %.1fs\n", localPath, elapsed.Seconds())
		}
	} else {
		// Write file directly
		dir := filepath.Dir(localPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			if !quiet {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Get file mode from header
		var mode os.FileMode = 0644 // default
		if modeStr := resp.Header.Get("X-File-Mode"); modeStr != "" {
			var modeVal uint32
			if _, err := fmt.Sscanf(modeStr, "%o", &modeVal); err == nil {
				mode = os.FileMode(modeVal)
			}
		}

		// Get original file size from header (for progress tracking)
		var totalSize int64
		if sizeStr := resp.Header.Get("X-Original-Size"); sizeStr != "" {
			fmt.Sscanf(sizeStr, "%d", &totalSize)
		}

		var f *os.File
		var written int64

		if offset > 0 {
			// Resume: open partial file for appending
			f, err = filetransfer.OpenPartialFileForAppend(localPath)
			if err != nil {
				if !quiet {
					fmt.Println("FAILED")
				}
				return fmt.Errorf("failed to open partial file: %w", err)
			}
			written = offset // Start counting from offset
		} else {
			// New download: create partial file
			if totalSize > 0 {
				f, err = filetransfer.CreatePartialFile(localPath, totalSize, remotePath, mode)
			} else {
				// No size info, create directly
				f, err = os.Create(filetransfer.GetPartialPath(localPath))
			}
			if err != nil {
				if !quiet {
					fmt.Println("FAILED")
				}
				return fmt.Errorf("failed to create file: %w", err)
			}
			if totalSize > 0 {
				// Partial info already created by CreatePartialFile
			}
		}

		// Copy data to file with progress tracking
		startTime := time.Now()
		if !quiet {
			fmt.Println() // Move to new line for progress bar
		}

		pw := &progressTrackingWriter{
			writer:    f,
			total:     totalSize,
			written:   &written,
			startTime: startTime,
			quiet:     quiet,
		}

		newBytes, err := io.Copy(pw, resp.Body)
		f.Close()

		if err != nil {
			// Update partial info with progress so far
			if totalSize > 0 {
				filetransfer.UpdatePartialProgress(localPath, written)
			}
			if !quiet {
				fmt.Print("\r") // Clear progress bar
			}
			fmt.Println("FAILED")
			return fmt.Errorf("failed to write file: %w", err)
		}

		// Finalize: rename partial to final
		if err := filetransfer.FinalizePartial(localPath, mode); err != nil {
			if !quiet {
				fmt.Print("\r") // Clear progress bar
			}
			fmt.Println("FAILED")
			return fmt.Errorf("failed to finalize file: %w", err)
		}

		// Clear progress bar and print final summary
		elapsed := time.Since(startTime)
		if !quiet && totalSize > 0 {
			fmt.Print("\r\033[K") // Clear line
		}

		speed := float64(newBytes) / elapsed.Seconds()
		if offset > 0 {
			fmt.Printf("Downloaded %s (resumed +%s) to %s in %.1fs (%s/s)\n",
				humanize.Bytes(uint64(written)), humanize.Bytes(uint64(newBytes)),
				localPath, elapsed.Seconds(), humanize.Bytes(uint64(speed)))
		} else {
			fmt.Printf("Downloaded %s to %s in %.1fs (%s/s)\n",
				humanize.Bytes(uint64(written)), localPath,
				elapsed.Seconds(), humanize.Bytes(uint64(speed)))
		}
	}

	return nil
}

func hashCmd() *cobra.Command {
	var cost int

	cmd := &cobra.Command{
		Use:   "hash [password]",
		Short: "Generate a bcrypt hash for use in configuration",
		Long: `Generate a bcrypt password hash for use in configuration files.

The generated hash can be used in:
  - socks5.auth.users[].password_hash  (SOCKS5 proxy authentication)
  - shell.password_hash                 (Shell command authentication)
  - file_transfer.password_hash         (File transfer authentication)

If no password is provided as an argument, you will be prompted to enter
it interactively (recommended for security).

Examples:
  # Interactive prompt (recommended - password hidden)
  muti-metroo hash

  # From argument (less secure - visible in shell history)
  muti-metroo hash "mysecretpassword"

  # With custom cost (default: 10, range: 4-31)
  muti-metroo hash --cost 12

  # Use in config file:
  # socks5:
  #   auth:
  #     enabled: true
  #     users:
  #       - username: admin
  #         password_hash: "<paste hash here>"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var password string

			if len(args) > 0 {
				password = args[0]
			} else {
				// Interactive prompt
				fmt.Print("Enter password: ")
				pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println() // newline after hidden input
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}

				fmt.Print("Confirm password: ")
				confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}

				if string(pwBytes) != string(confirmBytes) {
					return fmt.Errorf("passwords do not match")
				}

				password = string(pwBytes)
			}

			if password == "" {
				return fmt.Errorf("password cannot be empty")
			}

			// Validate cost
			if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
				return fmt.Errorf("cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
			}

			// Generate hash
			hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
			if err != nil {
				return fmt.Errorf("failed to generate hash: %w", err)
			}

			fmt.Println(string(hash))
			return nil
		},
	}

	cmd.Flags().IntVar(&cost, "cost", bcrypt.DefaultCost, "bcrypt cost factor (4-31, higher = slower but more secure)")

	return cmd
}

func managementKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "management-key",
		Short: "Manage mesh topology encryption keys",
		Long: `Manage X25519 keypairs for encrypting mesh topology data.

When management key encryption is enabled, sensitive data like NodeInfo
(hostnames, IPs, OS info) and route paths are encrypted before flooding
through the mesh. Only operators with the private key can decrypt and
view topology details.

This provides cryptographic compartmentalization: if a field agent is
compromised, the attacker only sees encrypted blobs, not the mesh topology.`,
	}

	// Add subcommands
	cmd.AddCommand(managementKeyGenerateCmd())
	cmd.AddCommand(managementKeyPublicCmd())

	return cmd
}

func managementKeyGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new management keypair",
		Long: `Generate a new X25519 keypair for management key encryption.

The generated keys should be distributed as follows:
  - Public key: Add to ALL agent configs (field agents and operators)
  - Private key: Add ONLY to operator/management node configs

Example output can be copied directly into your config.yaml:

  management:
    public_key: "<public key hex>"
    private_key: "<private key hex>"  # Only on operator nodes!`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Generate keypair using the identity package
			keypair, err := identity.NewKeypair()
			if err != nil {
				return fmt.Errorf("failed to generate keypair: %w", err)
			}

			pubKeyHex := hex.EncodeToString(keypair.PublicKey[:])
			privKeyHex := hex.EncodeToString(keypair.PrivateKey[:])

			fmt.Println("=== Management Keypair Generated ===")
			fmt.Println()
			fmt.Println("Public Key (add to ALL agent configs):")
			fmt.Printf("  %s\n", pubKeyHex)
			fmt.Println()
			fmt.Println("Private Key (add ONLY to operator configs - KEEP SECRET!):")
			fmt.Printf("  %s\n", privKeyHex)
			fmt.Println()
			fmt.Println("Config snippet for field agents:")
			fmt.Println("  management:")
			fmt.Printf("    public_key: \"%s\"\n", pubKeyHex)
			fmt.Println()
			fmt.Println("Config snippet for operator nodes:")
			fmt.Println("  management:")
			fmt.Printf("    public_key: \"%s\"\n", pubKeyHex)
			fmt.Printf("    private_key: \"%s\"\n", privKeyHex)

			return nil
		},
	}

	return cmd
}

func managementKeyPublicCmd() *cobra.Command {
	var privateKey string

	cmd := &cobra.Command{
		Use:   "public",
		Short: "Derive public key from private key",
		Long: `Derive the public key from an existing management private key.

This is useful if you've lost the public key but still have the private key,
or to verify that your keypair is consistent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if privateKey == "" {
				// Interactive prompt
				fmt.Print("Enter private key (hex): ")
				pkBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					return fmt.Errorf("failed to read private key: %w", err)
				}
				privateKey = string(pkBytes)
			}

			// Decode private key
			privBytes, err := hex.DecodeString(strings.TrimSpace(privateKey))
			if err != nil {
				return fmt.Errorf("invalid private key hex: %w", err)
			}

			if len(privBytes) != 32 {
				return fmt.Errorf("private key must be 32 bytes (64 hex chars), got %d bytes", len(privBytes))
			}

			// Use identity package to derive public key
			var privKey [32]byte
			copy(privKey[:], privBytes)

			pubKey := identity.DerivePublicKey(privKey)
			pubKeyHex := hex.EncodeToString(pubKey[:])

			fmt.Println("Public Key:")
			fmt.Printf("  %s\n", pubKeyHex)

			return nil
		},
	}

	cmd.Flags().StringVar(&privateKey, "private", "", "Private key in hex format")

	return cmd
}

// parseDuration parses a duration string (e.g., "5m", "30s") or plain seconds.
// Returns duration in seconds. Supports "0" for no timeout.
func parseDuration(s string) (int, error) {
	if s == "" || s == "0" {
		return 0, nil
	}

	// Try parsing as duration first (e.g., "5m", "30s", "1h")
	d, err := time.ParseDuration(s)
	if err == nil {
		return int(d.Seconds()), nil
	}

	// Fall back to parsing as integer seconds for backwards compatibility
	secs, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration '%s' (use format like '30s', '5m', or plain seconds)", s)
	}
	return secs, nil
}

// resolveAgentID resolves a short agent ID prefix to a full agent ID.
// If the ID is already 32 hex characters, it's returned as-is.
// Otherwise, it queries the /agents endpoint to find matching agents.
func resolveAgentID(shortID, agentAddr string) (string, error) {
	// Check if it's already a full ID (32 hex chars = 16 bytes = 128-bit AgentID)
	if len(shortID) == 32 && isHexString(shortID) {
		return shortID, nil
	}

	// Query the agents endpoint
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s/agents", agentAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// If we can't reach the API, assume the user provided a valid full ID
		return shortID, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return shortID, nil
	}

	var agents []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return shortID, nil
	}

	var matches []string
	for _, a := range agents {
		if strings.HasPrefix(strings.ToLower(a.ID), strings.ToLower(shortID)) {
			matches = append(matches, a.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no agent found matching prefix: %s", shortID)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix '%s' matches %d agents: %s...",
			shortID, len(matches), strings.Join(matches[:min(3, len(matches))], ", "))
	}
}

// isHexString checks if a string contains only hexadecimal characters.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Progress bar helper functions

// printProgress prints a progress bar to stdout.
func printProgress(current, total int64, startTime time.Time) {
	elapsed := time.Since(startTime).Seconds()
	if elapsed == 0 {
		elapsed = 0.001 // Avoid division by zero
	}
	speed := float64(current) / elapsed

	var pct float64
	if total > 0 {
		pct = float64(current) / float64(total) * 100
	}

	// Calculate ETA
	var eta string
	if speed > 0 && total > 0 {
		remaining := float64(total-current) / speed
		eta = formatProgressDuration(time.Duration(remaining) * time.Second)
	} else {
		eta = "--:--"
	}

	// Render simple ASCII bar: [=====>    ] 45% 1.2 MB/s ETA 2m30s
	bar := renderProgressBar(pct, 30)
	fmt.Printf("\r%s %.1f%% %s/s ETA %s  ", bar, pct, humanize.Bytes(uint64(speed)), eta)
}

// renderProgressBar renders an ASCII progress bar.
func renderProgressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled
	if filled > 0 {
		return "[" + strings.Repeat("=", filled-1) + ">" + strings.Repeat(" ", empty) + "]"
	}
	return "[" + strings.Repeat(" ", width) + "]"
}

// formatProgressDuration formats a duration for progress display.
func formatProgressDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// progressTrackingReader wraps an io.Reader and reports progress.
type progressTrackingReader struct {
	reader      io.Reader
	total       int64
	written     *int64
	startTime   time.Time
	quiet       bool
	lastPrinted time.Time
}

// Read implements io.Reader with progress tracking.
func (p *progressTrackingReader) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	if n > 0 {
		*p.written += int64(n)

		// Update progress bar (throttle to every 100ms to avoid flickering)
		if !p.quiet && time.Since(p.lastPrinted) > 100*time.Millisecond {
			printProgress(*p.written, p.total, p.startTime)
			p.lastPrinted = time.Now()
		}
	}
	return n, err
}

// progressTrackingWriter wraps an io.Writer and reports progress.
type progressTrackingWriter struct {
	writer      io.Writer
	total       int64
	written     *int64
	startTime   time.Time
	quiet       bool
	lastPrinted time.Time
}

// Write implements io.Writer with progress tracking.
func (p *progressTrackingWriter) Write(buf []byte) (int, error) {
	n, err := p.writer.Write(buf)
	if n > 0 {
		*p.written += int64(n)

		// Update progress bar (throttle to every 100ms to avoid flickering)
		if !p.quiet && time.Since(p.lastPrinted) > 100*time.Millisecond {
			printProgress(*p.written, p.total, p.startTime)
			p.lastPrinted = time.Now()
		}
	}
	return n, err
}

func licensesCmd() *cobra.Command {
	var format string
	var showFull bool

	cmd := &cobra.Command{
		Use:   "licenses",
		Short: "Show third-party license information",
		Long: `Display license information for all third-party dependencies included in this binary.

This command shows the licenses of all open-source libraries that Muti Metroo depends on.
All dependencies use permissive licenses (MIT, BSD-3-Clause, Apache-2.0, ISC).

Output formats:
  - table (default): Pretty table with Package and License columns
  - json: JSON array of {package, url, type}
  - csv: Raw CSV output (same format as embedded data)

Use --full to append the complete license texts after the summary.

Examples:
  # Show license summary in table format
  muti-metroo licenses

  # Output as JSON
  muti-metroo licenses --format json

  # Show all license texts
  muti-metroo licenses --full

  # Export to file
  muti-metroo licenses --format csv > licenses.csv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			licList, err := licenses.List()
			if err != nil {
				return fmt.Errorf("failed to load licenses: %w", err)
			}

			switch format {
			case "table":
				fmt.Printf("Third-Party Licenses\n")
				fmt.Printf("====================\n\n")

				// Calculate column widths
				maxPkg := 40
				for _, lic := range licList {
					if len(lic.Package) > maxPkg {
						maxPkg = len(lic.Package)
					}
				}
				if maxPkg > 60 {
					maxPkg = 60 // Cap width
				}

				fmt.Printf("%-*s  %s\n", maxPkg, "Package", "License")
				fmt.Printf("%-*s  %s\n", maxPkg, strings.Repeat("-", maxPkg), "-------")

				for _, lic := range licList {
					pkg := lic.Package
					if len(pkg) > maxPkg {
						pkg = pkg[:maxPkg-3] + "..."
					}
					fmt.Printf("%-*s  %s\n", maxPkg, pkg, lic.Type)
				}

				fmt.Printf("\nTotal: %d dependencies\n", len(licList))

				// Show license type summary
				types := licenses.LicenseTypes()
				var summary []string
				for t, count := range types {
					summary = append(summary, fmt.Sprintf("%s: %d", t, count))
				}
				fmt.Printf("License types: %s\n", strings.Join(summary, ", "))

				if !showFull {
					fmt.Printf("\nUse --full to see complete license texts.\n")
				}

			case "json":
				type licenseJSON struct {
					Package string `json:"package"`
					URL     string `json:"url"`
					Type    string `json:"type"`
				}
				list := make([]licenseJSON, len(licList))
				for i, lic := range licList {
					list[i] = licenseJSON{
						Package: lic.Package,
						URL:     lic.URL,
						Type:    lic.Type,
					}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(list)

			case "csv":
				fmt.Print(string(licenses.LicensesCSV))
				return nil

			default:
				return fmt.Errorf("unknown format: %s (use table, json, or csv)", format)
			}

			if showFull {
				fmt.Printf("\n")
				text, err := licenses.GetAllLicenseTexts()
				if err != nil {
					return fmt.Errorf("failed to get license texts: %w", err)
				}
				fmt.Print(text)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format: table, json, csv")
	cmd.Flags().BoolVar(&showFull, "full", false, "Show full license texts")

	return cmd
}
