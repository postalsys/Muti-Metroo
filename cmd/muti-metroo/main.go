// Package main provides the CLI entry point for Muti Metroo mesh agent.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/postalsys/muti-metroo/internal/agent"
	"github.com/postalsys/muti-metroo/internal/certutil"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/control"
	"github.com/postalsys/muti-metroo/internal/filetransfer"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/rpc"
	"github.com/postalsys/muti-metroo/internal/service"
	"github.com/postalsys/muti-metroo/internal/wizard"
	"github.com/spf13/cobra"
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

	// Add subcommands
	rootCmd.AddCommand(setupCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(certCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(peersCmd())
	rootCmd.AddCommand(routesCmd())
	rootCmd.AddCommand(uninstallCmd())
	rootCmd.AddCommand(rpcCmd())
	rootCmd.AddCommand(uploadCmd())
	rootCmd.AddCommand(downloadCmd())

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
	var socketPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agent status",
		Long:  "Display the current status of the running agent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := control.NewClient(socketPath)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			status, err := client.Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			fmt.Printf("Agent Status\n")
			fmt.Printf("============\n")
			fmt.Printf("Agent ID:    %s\n", status.AgentID)
			fmt.Printf("Running:     %v\n", status.Running)
			fmt.Printf("Peer Count:  %d\n", status.PeerCount)
			fmt.Printf("Route Count: %d\n", status.RouteCount)

			return nil
		},
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "./data/control.sock", "Path to control socket")

	return cmd
}

func peersCmd() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "peers",
		Short: "List connected peers",
		Long:  "Display all peers currently connected to this agent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := control.NewClient(socketPath)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			peers, err := client.Peers(ctx)
			if err != nil {
				return fmt.Errorf("failed to get peers: %w", err)
			}

			fmt.Printf("Connected Peers\n")
			fmt.Printf("===============\n")
			if len(peers.Peers) == 0 {
				fmt.Println("No peers connected.")
			} else {
				for i, peerID := range peers.Peers {
					fmt.Printf("%d. %s\n", i+1, peerID)
				}
				fmt.Printf("\nTotal: %d peer(s)\n", len(peers.Peers))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "./data/control.sock", "Path to control socket")

	return cmd
}

func routesCmd() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "routes",
		Short: "List route table",
		Long:  "Display the current routing table.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := control.NewClient(socketPath)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			routes, err := client.Routes(ctx)
			if err != nil {
				return fmt.Errorf("failed to get routes: %w", err)
			}

			fmt.Printf("Route Table\n")
			fmt.Printf("===========\n")
			if len(routes.Routes) == 0 {
				fmt.Println("No routes in table.")
			} else {
				fmt.Printf("%-20s %-12s %-12s %-8s %-6s\n", "NETWORK", "NEXT HOP", "ORIGIN", "METRIC", "HOPS")
				fmt.Printf("%-20s %-12s %-12s %-8s %-6s\n", "-------", "--------", "------", "------", "----")
				for _, route := range routes.Routes {
					fmt.Printf("%-20s %-12s %-12s %-8d %-6d\n",
						route.Network,
						route.NextHop,
						route.Origin,
						route.Metric,
						route.HopCount,
					)
				}
				fmt.Printf("\nTotal: %d route(s)\n", len(routes.Routes))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "./data/control.sock", "Path to control socket")

	return cmd
}

func uninstallCmd() *cobra.Command {
	var serviceName string
	var force bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the system service",
		Long: `Remove the Muti Metroo system service.

On Linux, this stops and removes the systemd service.
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
				case "linux":
					return fmt.Errorf("must run as root to uninstall the service (try: sudo muti-metroo uninstall)")
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

	cmd.Flags().StringVarP(&commonName, "cn", "n", "Muti Metroo CA", "Common name for the CA")
	cmd.Flags().StringVarP(&outDir, "out", "o", "./certs", "Output directory for certificate files")
	cmd.Flags().IntVarP(&validDays, "days", "d", 365, "Validity period in days")

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

	cmd.Flags().StringVarP(&commonName, "cn", "n", "", "Common name for the certificate (required)")
	cmd.Flags().StringVarP(&outDir, "out", "o", "./certs", "Output directory for certificate files")
	cmd.Flags().IntVarP(&validDays, "days", "d", 90, "Validity period in days")
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

	cmd.Flags().StringVarP(&commonName, "cn", "n", "", "Common name for the certificate (required)")
	cmd.Flags().StringVarP(&outDir, "out", "o", "./certs", "Output directory for certificate files")
	cmd.Flags().IntVarP(&validDays, "days", "d", 90, "Validity period in days")
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

func rpcCmd() *cobra.Command {
	var (
		agentAddr string
		password  string
		timeout   int
	)

	cmd := &cobra.Command{
		Use:   "rpc [flags] <target-agent-id> <command> [args...]",
		Short: "Execute a command on a remote agent",
		Long: `Execute a shell command on a remote agent via the RPC interface.

The command is sent through a local or remote agent's health HTTP server
to the target agent identified by its agent ID.

Stdin is forwarded to the remote command if provided via pipe.

Examples:
  # Run whoami on a remote agent (via localhost:8080)
  muti-metroo rpc abc123def456 whoami

  # Run with arguments
  muti-metroo rpc abc123def456 ls -la /tmp

  # Via a different agent
  muti-metroo rpc -a 192.168.1.10:8080 abc123def456 hostname

  # With password authentication
  muti-metroo rpc -p secret abc123def456 ip addr

  # Pipe stdin to remote command
  echo "hello" | muti-metroo rpc abc123def456 cat`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID := args[0]
			command := args[1]
			cmdArgs := args[2:]

			// Validate target agent ID
			if _, err := identity.ParseAgentID(targetID); err != nil {
				return fmt.Errorf("invalid agent ID '%s': %w", targetID, err)
			}

			// Read stdin if available (non-blocking check)
			var stdinData []byte
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				// Data is being piped in
				var err error
				stdinData, err = io.ReadAll(io.LimitReader(os.Stdin, rpc.MaxStdinSize))
				if err != nil {
					return fmt.Errorf("failed to read stdin: %w", err)
				}
			}

			// Build request
			reqBody := map[string]interface{}{
				"command": command,
				"args":    cmdArgs,
				"timeout": timeout,
			}
			if password != "" {
				reqBody["password"] = password
			}
			if len(stdinData) > 0 {
				reqBody["stdin"] = rpc.EncodeBase64(stdinData)
			}

			reqJSON, err := json.Marshal(reqBody)
			if err != nil {
				return fmt.Errorf("failed to encode request: %w", err)
			}

			// Build URL
			url := fmt.Sprintf("http://%s/agents/%s/rpc", agentAddr, targetID)

			// Create HTTP request
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+30)*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqJSON))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to send request: %w", err)
			}
			defer resp.Body.Close()

			// Read response
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			// Parse response
			var rpcResp rpc.Response
			if err := json.Unmarshal(respBody, &rpcResp); err != nil {
				return fmt.Errorf("failed to parse response: %w (body: %s)", err, string(respBody))
			}

			// Output stdout (decode from base64 if encoded)
			if rpcResp.Stdout != "" {
				if decoded, err := rpc.DecodeBase64(rpcResp.Stdout); err == nil {
					fmt.Print(string(decoded))
				} else {
					fmt.Print(rpcResp.Stdout)
				}
			}

			// Output stderr to stderr (decode from base64 if encoded)
			if rpcResp.Stderr != "" {
				if decoded, err := rpc.DecodeBase64(rpcResp.Stderr); err == nil {
					fmt.Fprint(os.Stderr, string(decoded))
				} else {
					fmt.Fprint(os.Stderr, rpcResp.Stderr)
				}
			}

			// Output error if present
			if rpcResp.Error != "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n", rpcResp.Error)
			}

			// Exit with remote exit code
			if rpcResp.ExitCode != 0 {
				os.Exit(rpcResp.ExitCode)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Agent health server address (host:port)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "RPC password for authentication")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 60, "Command timeout in seconds")

	return cmd
}

func uploadCmd() *cobra.Command {
	var (
		agentAddr string
		password  string
		timeout   int
	)

	cmd := &cobra.Command{
		Use:   "upload [flags] <target-agent-id> <local-path> <remote-path>",
		Short: "Upload a file to a remote agent",
		Long: `Upload a local file to a remote agent via the file transfer interface.

The file is uploaded through a local or remote agent's health HTTP server
to the target agent identified by its agent ID.

File permissions (mode) are preserved. The remote path must be absolute.

Examples:
  # Upload a file to a remote agent
  muti-metroo upload abc123def456 ./local/file.txt /tmp/remote-file.txt

  # Via a different agent
  muti-metroo upload -a 192.168.1.10:8080 abc123def456 config.yaml /etc/app/config.yaml

  # With password authentication
  muti-metroo upload -p secret abc123def456 ./data.bin /home/user/data.bin`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID := args[0]
			localPath := args[1]
			remotePath := args[2]

			// Validate target agent ID
			if _, err := identity.ParseAgentID(targetID); err != nil {
				return fmt.Errorf("invalid agent ID '%s': %w", targetID, err)
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

			// Check if local file exists
			info, err := os.Stat(absLocalPath)
			if err != nil {
				return fmt.Errorf("cannot access local file: %w", err)
			}
			if info.IsDir() {
				return fmt.Errorf("local path is a directory: %s", localPath)
			}

			fileSize := info.Size()
			fileName := filepath.Base(localPath)

			fmt.Printf("Uploading %s (%d bytes) to %s:%s\n", fileName, fileSize, targetID[:12], remotePath)

			// Read and encode file
			data, mode, size, compressed, err := filetransfer.ReadFileForTransfer(absLocalPath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			// Build request
			reqBody := filetransfer.FileUploadRequest{
				Password:   password,
				Path:       remotePath,
				Mode:       mode,
				Size:       size,
				Compressed: compressed,
				Data:       data,
			}

			reqJSON, err := json.Marshal(reqBody)
			if err != nil {
				return fmt.Errorf("failed to encode request: %w", err)
			}

			// Build URL
			url := fmt.Sprintf("http://%s/agents/%s/file/upload", agentAddr, targetID)

			// Create HTTP request
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqJSON))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			// Show progress indicator
			fmt.Print("Uploading... ")

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to send request: %w", err)
			}
			defer resp.Body.Close()

			// Read response
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to read response: %w", err)
			}

			// Parse response
			var uploadResp filetransfer.FileUploadResponse
			if err := json.Unmarshal(respBody, &uploadResp); err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to parse response: %w (body: %s)", err, string(respBody))
			}

			if !uploadResp.Success {
				fmt.Println("FAILED")
				return fmt.Errorf("upload failed: %s", uploadResp.Error)
			}

			fmt.Println("OK")
			fmt.Printf("Uploaded %d bytes to %s\n", uploadResp.Written, remotePath)
			fmt.Printf("Mode: %04o\n", mode)

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Agent health server address (host:port)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "File transfer password for authentication")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 300, "Transfer timeout in seconds")

	return cmd
}

func downloadCmd() *cobra.Command {
	var (
		agentAddr string
		password  string
		timeout   int
	)

	cmd := &cobra.Command{
		Use:   "download [flags] <target-agent-id> <remote-path> <local-path>",
		Short: "Download a file from a remote agent",
		Long: `Download a file from a remote agent via the file transfer interface.

The file is downloaded through a local or remote agent's health HTTP server
from the target agent identified by its agent ID.

File permissions (mode) are preserved. The remote path must be absolute.

Examples:
  # Download a file from a remote agent
  muti-metroo download abc123def456 /tmp/remote-file.txt ./local/file.txt

  # Via a different agent
  muti-metroo download -a 192.168.1.10:8080 abc123def456 /etc/app/config.yaml config.yaml

  # With password authentication
  muti-metroo download -p secret abc123def456 /home/user/data.bin ./data.bin`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID := args[0]
			remotePath := args[1]
			localPath := args[2]

			// Validate target agent ID
			if _, err := identity.ParseAgentID(targetID); err != nil {
				return fmt.Errorf("invalid agent ID '%s': %w", targetID, err)
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

			fmt.Printf("Downloading %s:%s to %s\n", targetID[:12], remotePath, localPath)

			// Build request
			reqBody := filetransfer.FileDownloadRequest{
				Password: password,
				Path:     remotePath,
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

			// Show progress indicator
			fmt.Print("Downloading... ")

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to send request: %w", err)
			}
			defer resp.Body.Close()

			// Read response
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to read response: %w", err)
			}

			// Parse response
			var downloadResp filetransfer.FileDownloadResponse
			if err := json.Unmarshal(respBody, &downloadResp); err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to parse response: %w (body: %s)", err, string(respBody))
			}

			if !downloadResp.Success {
				fmt.Println("FAILED")
				return fmt.Errorf("download failed: %s", downloadResp.Error)
			}

			fmt.Println("OK")

			// Write file
			written, err := filetransfer.WriteFileFromTransfer(absLocalPath, downloadResp.Data, downloadResp.Mode, downloadResp.Compressed)
			if err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Downloaded %d bytes to %s\n", written, localPath)
			fmt.Printf("Mode: %04o\n", downloadResp.Mode)

			return nil
		},
	}

	cmd.Flags().StringVarP(&agentAddr, "agent", "a", "localhost:8080", "Agent health server address (host:port)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "File transfer password for authentication")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 300, "Transfer timeout in seconds")

	return cmd
}
