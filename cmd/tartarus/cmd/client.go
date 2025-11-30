package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
)

func getHTTPClient() *http.Client {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecure,
	}

	// Load client cert if provided
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading client certificate: %v\n", err)
			os.Exit(1)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA if provided
	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading CA file: %v\n", err)
			os.Exit(1)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	return &http.Client{
		Transport: transport,
	}
}

func doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", host, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else if envKey := os.Getenv("TARTARUS_API_KEY"); envKey != "" {
		req.Header.Set("Authorization", "Bearer "+envKey)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := getHTTPClient()
	return client.Do(req)
}
