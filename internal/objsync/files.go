package objsync

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// ErrFileTooLarge is returned when a source file exceeds the configured size cap.
type ErrFileTooLarge struct {
	Field string
	URL   string
	Size  int64 // 0 when unknown (LimitReader tripped without Content-Length)
	Max   int64
}

func (e *ErrFileTooLarge) Error() string {
	if e.Size > 0 {
		return fmt.Sprintf("file field %q exceeds max size: %d bytes > %d bytes limit (%s)", e.Field, e.Size, e.Max, e.URL)
	}
	return fmt.Sprintf("file field %q exceeds max size: > %d bytes limit (%s)", e.Field, e.Max, e.URL)
}

func IsErrFileTooLarge(err error) bool {
	var e *ErrFileTooLarge
	return errors.As(err, &e)
}

// fileHTTPClient builds a client for downloading source file URLs. It applies
// insecureSkipVerify from the source app (file hosts often share that TLS trust).
// Unlike the API client, it does not remap dial Host/Port — storage URLs are absolute.
func fileHTTPClient(insecureSkipVerify bool) *http.Client {
	c := &http.Client{Timeout: 120 * time.Second}
	if !insecureSkipVerify {
		return c
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	c.Transport = transport
	return c
}

// fileFieldURL returns the http(s) URL string for a file field value, or "" if empty/null.
func fileFieldURL(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected URL string, got %T", v)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return "", fmt.Errorf("expected http(s) URL, got %q", s)
	}
	return s, nil
}

func filenameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "file"
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "file"
	}
	return base
}

// preflightFileSizes checks Content-Length via HEAD (or GET on HEAD failure) for
// each configured file field. Oversized files return *ErrFileTooLarge without
// reading bodies into memory.
func preflightFileSizes(client *http.Client, row map[string]any, fileFields []string, maxBytes int64) error {
	if len(fileFields) == 0 || maxBytes <= 0 {
		return nil
	}
	if client == nil {
		client = fileHTTPClient(false)
	}
	for _, field := range fileFields {
		rawURL, err := fileFieldURL(row[field])
		if err != nil {
			return fmt.Errorf("field %q: %w", field, err)
		}
		if rawURL == "" {
			continue
		}
		size, ok, err := probeContentLength(client, rawURL)
		if err != nil {
			return fmt.Errorf("field %q: %w", field, err)
		}
		if ok && size > maxBytes {
			return &ErrFileTooLarge{Field: field, URL: rawURL, Size: size, Max: maxBytes}
		}
	}
	return nil
}

func probeContentLength(client *http.Client, rawURL string) (size int64, ok bool, err error) {
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		return probeContentLengthGET(client, rawURL)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, false, fmt.Errorf("HEAD %s: status %s", rawURL, resp.Status)
	}
	if resp.ContentLength >= 0 {
		return resp.ContentLength, true, nil
	}
	return 0, false, nil
}

// probeContentLengthGET issues a GET and closes the body without reading it when
// Content-Length is known; used when HEAD is unsupported.
func probeContentLengthGET(client *http.Client, rawURL string) (size int64, ok bool, err error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, false, fmt.Errorf("GET %s: status %s", rawURL, resp.Status)
	}
	if resp.ContentLength >= 0 {
		return resp.ContentLength, true, nil
	}
	return 0, false, nil
}

// downloadFilePayload GETs a single file URL and returns Bobuild's {data, name}
// map. Reads at most maxBytes+1 via LimitReader so oversized bodies cannot OOM.
func downloadFilePayload(client *http.Client, field, rawURL string, maxBytes int64) (map[string]any, error) {
	if client == nil {
		client = fileHTTPClient(false)
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: status %s", rawURL, resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return nil, &ErrFileTooLarge{Field: field, URL: rawURL, Size: resp.ContentLength, Max: maxBytes}
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, &ErrFileTooLarge{Field: field, URL: rawURL, Size: 0, Max: maxBytes}
	}

	return map[string]any{
		"data": base64.StdEncoding.EncodeToString(data),
		"name": filenameFromURL(rawURL),
	}, nil
}
