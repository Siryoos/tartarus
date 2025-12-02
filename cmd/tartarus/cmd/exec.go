package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var execCmd = &cobra.Command{
	Use:   "exec [sandbox-id] [command...]",
	Short: "Execute a command in a running sandbox",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		command := args[1:]

		if interactive {
			runInteractive(cmd, id, command)
			return
		}

		reqBody := struct {
			Cmd []string `json:"cmd"`
		}{
			Cmd: command,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling request: %v\n", err)
			os.Exit(1)
		}

		path := fmt.Sprintf("/sandboxes/%s/exec", id)
		resp, err := doRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			fmt.Fprintf(os.Stderr, "Error executing command: status %d\n", resp.StatusCode)
			os.Exit(1)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Exec command requested")
	},
}

func runInteractive(cmd *cobra.Command, id string, command []string) {
	// Construct WebSocket URL
	// host is global var from root.go, e.g. http://localhost:8080
	// We need to convert http -> ws, https -> wss
	u, err := url.Parse(host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid host URL: %v\n", err)
		os.Exit(1)
	}

	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}

	path := fmt.Sprintf("/sandboxes/exec/sock/%s", id)
	wsURL := url.URL{Scheme: scheme, Host: u.Host, Path: path}

	// Add command as query param
	q := wsURL.Query()
	q.Set("cmd", strings.Join(command, " "))
	wsURL.RawQuery = q.Encode()

	fmt.Printf("Connecting to %s...\n", wsURL.String())

	c, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	// Handle terminal raw mode
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to set raw mode: %v\n", err)
		} else {
			defer term.Restore(fd, oldState)
		}
	}

	// Handle SIGINT to restore terminal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		c.Close()
		os.Exit(0)
	}()

	done := make(chan struct{})

	// Read from WS -> Stdout
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				// Normal closure?
				return
			}
			os.Stdout.Write(message)
		}
	}()

	// Read from Stdin -> WS
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if err := c.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	<-done
}

var interactive bool

func init() {
	execCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive mode")
	rootCmd.AddCommand(execCmd)
}
