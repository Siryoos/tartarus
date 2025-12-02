package cerberus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// CertWatcher watches certificate files and updates TLS configuration dynamically.
type CertWatcher struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	ClientAuth tls.ClientAuthType
	Logger     *slog.Logger

	mu        sync.RWMutex
	cert      *tls.Certificate
	clientCAs *x509.CertPool
}

// NewCertWatcher creates a new CertWatcher.
func NewCertWatcher(certFile, keyFile, caFile string, clientAuth tls.ClientAuthType, logger *slog.Logger) (*CertWatcher, error) {
	w := &CertWatcher{
		CertFile:   certFile,
		KeyFile:    keyFile,
		CAFile:     caFile,
		ClientAuth: clientAuth,
		Logger:     logger,
	}

	if err := w.reload(); err != nil {
		return nil, err
	}

	return w, nil
}

// reload reads the files and updates the state.
func (w *CertWatcher) reload() error {
	var cert *tls.Certificate
	var clientCAs *x509.CertPool

	// Load Server Cert/Key
	if w.CertFile != "" && w.KeyFile != "" {
		c, err := tls.LoadX509KeyPair(w.CertFile, w.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load key pair: %w", err)
		}
		cert = &c
	}

	// Load CA Bundle
	if w.CAFile != "" {
		caBytes, err := os.ReadFile(w.CAFile)
		if err != nil {
			return fmt.Errorf("failed to read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return fmt.Errorf("failed to append CA certs")
		}
		clientCAs = pool
	}

	w.mu.Lock()
	w.cert = cert
	w.clientCAs = clientCAs
	w.mu.Unlock()

	w.Logger.Info("Reloaded TLS certificates", "ca_file", w.CAFile)
	return nil
}

// Start starts the watcher loop.
func (w *CertWatcher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.reload(); err != nil {
				w.Logger.Error("Failed to reload certificates", "error", err)
			}
		}
	}
}

// GetCertificate is a callback for tls.Config.GetCertificate.
func (w *CertWatcher) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cert, nil
}

// GetConfigForClient is a callback for tls.Config.GetConfigForClient.
func (w *CertWatcher) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Return a config with the current CAs
	return &tls.Config{
		Certificates: []tls.Certificate{*w.cert},
		ClientCAs:    w.clientCAs,
		ClientAuth:   w.ClientAuth,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// TLSConfig returns a base tls.Config that uses the watcher callbacks.
func (w *CertWatcher) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate:     w.GetCertificate,
		GetConfigForClient: w.GetConfigForClient,
		ClientAuth:         w.ClientAuth,
		MinVersion:         tls.VersionTLS12,
		// We set ClientCAs initially too for non-SNI clients or initial handshake?
		// GetConfigForClient overrides it for the handshake.
	}
}
