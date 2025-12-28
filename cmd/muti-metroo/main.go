// Package main provides the CLI entry point for Muti Metroo mesh agent.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coinstash/muti-metroo/internal/agent"
	"github.com/coinstash/muti-metroo/internal/certutil"
	"github.com/coinstash/muti-metroo/internal/config"
	"github.com/coinstash/muti-metroo/internal/control"
	"github.com/coinstash/muti-metroo/internal/identity"
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
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(certCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(peersCmd())
	rootCmd.AddCommand(routesCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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

			fmt.Printf("Starting Muti Metroo agent...\n")
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
