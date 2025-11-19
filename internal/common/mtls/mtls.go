package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// Config holds mTLS configuration
type Config struct {
	Enabled    bool
	CACert     string // Path to CA certificate
	ServerCert string // Path to server certificate
	ServerKey  string // Path to server private key
	ClientCert string // Path to client certificate (for outgoing requests)
	ClientKey  string // Path to client private key (for outgoing requests)
}

// LoadFromEnv loads mTLS configuration from environment variables
func LoadFromEnv() *Config {
	enabled := os.Getenv("MTLS_ENABLED") == "true"
	
	return &Config{
		Enabled:    enabled,
		CACert:     os.Getenv("MTLS_CA_CERT"),
		ServerCert: os.Getenv("MTLS_SERVER_CERT"),
		ServerKey:  os.Getenv("MTLS_SERVER_KEY"),
		ClientCert: os.Getenv("MTLS_CLIENT_CERT"),
		ClientKey:  os.Getenv("MTLS_CLIENT_KEY"),
	}
}

// ServerTLSConfig creates TLS config for HTTP server
// This validates client certificates
func (c *Config) ServerTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	// Load CA certificate
	caCert, err := os.ReadFile(c.CACert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	// Create CA cert pool
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	// Load server certificate and key
	serverCert, err := tls.LoadX509KeyPair(c.ServerCert, c.ServerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load server cert: %w", err)
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		// Server certificate
		Certificates: []tls.Certificate{serverCert},
		
		// Client certificate validation
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caCertPool,
		
		// Security settings
		MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.CurveP521,
			tls.CurveP384,
			tls.CurveP256,
		},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	return tlsConfig, nil
}

// ClientTLSConfig creates TLS config for HTTP client
// This presents client certificate to servers
func (c *Config) ClientTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	// Load CA certificate
	caCert, err := os.ReadFile(c.CACert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	// Create CA cert pool
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	// Load client certificate and key
	clientCert, err := tls.LoadX509KeyPair(c.ClientCert, c.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert: %w", err)
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		// Client certificate
		Certificates: []tls.Certificate{clientCert},
		
		// Server certificate validation
		RootCAs: caCertPool,
		
		// Security settings
		MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.CurveP521,
			tls.CurveP384,
			tls.CurveP256,
		},
	}

	return tlsConfig, nil
}

// VerifyPeerCertificate validates the peer's certificate
// This can be used for additional custom validation
func VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// Custom validation logic can go here
	// For example, check CN, organization, etc.
	
	if len(verifiedChains) == 0 {
		return fmt.Errorf("no verified certificate chains")
	}

	// Get the peer certificate
	cert := verifiedChains[0][0]
	
	// Example: Validate organization
	if len(cert.Subject.Organization) > 0 && cert.Subject.Organization[0] != "Mercuria" {
		return fmt.Errorf("invalid organization: %s", cert.Subject.Organization[0])
	}

	return nil
}