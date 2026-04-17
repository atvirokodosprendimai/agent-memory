// Package ipfs provides a client for interacting with a Kubo IPFS daemon
// via its HTTP RPC API.
package ipfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Client wraps an HTTP client for communicating with a Kubo IPFS daemon.
type Client struct {
	addr   string
	client *http.Client
}

// NewClient creates a new IPFS RPC client. The addr should be the daemon's
// API endpoint, e.g. "http://localhost:5001".
func NewClient(addr string) *Client {
	return &Client{
		addr:   addr,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// post sends a multipart form POST to the given API path with optional file
// data. If filename and data are provided, they are included as a file part
// named "file".
func (c *Client) post(path string, filename string, data []byte) (*http.Response, error) {
	var body bytes.Buffer

	if data != nil {
		w := multipart.NewWriter(&body)
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		if _, err := part.Write(data); err != nil {
			w.Close()
			return nil, fmt.Errorf("writing form file: %w", err)
		}
		w.Close()

		req, err := http.NewRequest(http.MethodPost, c.addr+path, &body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		return c.client.Do(req)
	}

	// No file data — send an empty multipart form so the RPC is happy.
	w := multipart.NewWriter(&body)
	w.Close()

	req, err := http.NewRequest(http.MethodPost, c.addr+path, &body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.client.Do(req)
}

// Ping checks whether the IPFS daemon is reachable by calling /api/v0/id.
func (c *Client) Ping() error {
	resp, err := c.post("/api/v0/id", "", nil)
	if err != nil {
		return fmt.Errorf("ipfs daemon unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ipfs ping failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// addResponse represents the JSON response from /api/v0/add.
type addResponse struct {
	Name string `json:"Name"`
	Hash string `json:"Hash"`
	Size string `json:"Size"`
}

// Add uploads the given data to IPFS and returns the CID of the pinned
// content. Kubo pins content automatically when adding.
func (c *Client) Add(data []byte) (string, error) {
	resp, err := c.post("/api/v0/add", "file", data)
	if err != nil {
		return "", fmt.Errorf("add request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("add failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// The response may contain multiple JSON lines (one per file). We only
	// need the last one which contains the directory/root hash when adding
	// a single file.
	var result addResponse
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var entry addResponse
		if err := decoder.Decode(&entry); err != nil {
			return "", fmt.Errorf("decoding add response: %w", err)
		}
		result = entry
	}

	if result.Hash == "" {
		return "", fmt.Errorf("add response contained no hash")
	}
	return result.Hash, nil
}

// Get retrieves the raw bytes of the content identified by the given CID.
func (c *Client) Get(cid string) ([]byte, error) {
	resp, err := c.post("/api/v0/cat?arg="+cid, "", nil)
	if err != nil {
		return nil, fmt.Errorf("cat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cat failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// pinLsResponse represents the JSON response from /api/v0/pin/ls.
type pinLsResponse struct {
	Keys map[string]struct {
		Type string `json:"Type"`
	} `json:"Keys"`
}

// PinLs returns a map of all pinned CIDs. The value is always true.
func (c *Client) PinLs() (map[string]bool, error) {
	resp, err := c.post("/api/v0/pin/ls", "", nil)
	if err != nil {
		return nil, fmt.Errorf("pin/ls request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pin/ls failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result pinLsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding pin/ls response: %w", err)
	}

	pins := make(map[string]bool, len(result.Keys))
	for cid := range result.Keys {
		pins[cid] = true
	}
	return pins, nil
}

// PinRm removes the pin for the given CID.
func (c *Client) PinRm(cid string) error {
	resp, err := c.post("/api/v0/pin/rm?arg="+cid, "", nil)
	if err != nil {
		return fmt.Errorf("pin/rm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pin/rm failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close closes the underlying HTTP client and releases resources.
func (c *Client) Close() error {
	c.client.CloseIdleConnections()
	return nil
}
