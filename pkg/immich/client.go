package immich

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Verbose    bool
	LogWriter  io.Writer
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: time.Second * 30,
		},
	}
}

func (c *Client) doRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	var requestBody []byte
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = buf
		bodyReader = bytes.NewReader(buf)
	}

	requestURL := fmt.Sprintf("%s%s", c.BaseURL, endpoint)
	req, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	c.logRequest(method, requestURL, requestBody)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		c.logError("%s %s: request failed: %v", method, requestURL, err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logResponse(method, requestURL, resp.StatusCode, respBody)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API Error: %s (Status: %d) Endpoint: %s", string(respBody), resp.StatusCode, endpoint)
	}

	return respBody, nil
}

func (c *Client) verbosef(format string, args ...interface{}) {
	if !c.Verbose {
		return
	}
	log.New(c.logWriter(), "Immich: ", 0).Printf(format, args...)
}

func (c *Client) logWriter() io.Writer {
	if c.LogWriter != nil {
		return c.LogWriter
	}
	return os.Stderr
}

func (c *Client) logRequest(method, requestURL string, body []byte) {
	if len(body) == 0 {
		c.verbosef("--> %s %s (no body)", method, requestURL)
		return
	}
	c.verbosef("--> %s %s\n%s", method, requestURL, prettyJSON(body))
}

func (c *Client) logResponse(method, requestURL string, status int, body []byte) {
	if len(body) == 0 {
		c.verbosef("<-- %d %s %s (empty body)", status, method, requestURL)
		return
	}
	c.verbosef("<-- %d %s %s\n%s", status, method, requestURL, prettyJSON(body))
}

func (c *Client) logError(format string, args ...interface{}) {
	c.verbosef("!! "+format, args...)
}

// prettyJSON indents a JSON payload for readable verbose output, falling back to
// the raw bytes when the payload is not valid JSON.
func prettyJSON(body []byte) string {
	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "  "); err != nil {
		return string(body)
	}
	return indented.String()
}
