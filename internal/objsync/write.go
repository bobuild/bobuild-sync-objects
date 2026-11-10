package objsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	bobuild "github.com/bobuild/bobuild-client-go"
)

type writeResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func apiURL(c *bobuild.Client, endpoint string) string {
	scheme := "https"
	if !c.UseTLS {
		scheme = "http"
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return scheme + "://" + c.Domain + "/_api" + endpoint
}

// doWrite performs POST/PATCH/DELETE without requiring a typed id in the
// response (Bobuild may return id as number or string on create).
func doWrite(c *bobuild.Client, method, endpoint string, payload any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequest(method, apiURL(c, endpoint), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API call failed with status: %s %s", resp.Status, raw)
	}

	var parsed writeResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parse response: %w (%s)", err, raw)
	}
	if parsed.Error != "" {
		return fmt.Errorf("API error: %s", parsed.Error)
	}
	if !parsed.Success {
		return fmt.Errorf("API reported success=false: %s", raw)
	}
	return nil
}

func createRecord(c *bobuild.Client, endpoint string, payload map[string]any) error {
	return doWrite(c, http.MethodPost, endpoint, payload)
}

// payloadWithoutID copies payload omitting id. Modify endpoints take the
// record id from the URL path and reject id in the JSON body ("Unexpected keys").
func payloadWithoutID(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if k == "id" {
			continue
		}
		out[k] = v
	}
	return out
}

func modifyRecord(c *bobuild.Client, endpoint string, payload map[string]any) error {
	return doWrite(c, http.MethodPatch, endpoint, payloadWithoutID(payload))
}

func deleteRecord(c *bobuild.Client, endpoint string) error {
	return doWrite(c, http.MethodDelete, endpoint, nil)
}
