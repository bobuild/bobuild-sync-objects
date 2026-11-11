package objsync

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileHTTPClientInsecureSkipVerify(t *testing.T) {
	c := fileHTTPClient(true)
	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify transport, got %#v", c.Transport)
	}
	c2 := fileHTTPClient(false)
	if c2.Transport != nil {
		t.Fatalf("expected default transport unset, got %#v", c2.Transport)
	}
}

func TestFilenameFromURL(t *testing.T) {
	if got := filenameFromURL("https://cdn.example/path/song_1_cover.jpg?sig=1"); got != "song_1_cover.jpg" {
		t.Fatalf("got %q", got)
	}
	if got := filenameFromURL("https://cdn.example/"); got != "file" {
		t.Fatalf("got %q", got)
	}
}

func TestDownloadFilePayload(t *testing.T) {
	body := []byte("hello-file")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, err := downloadFilePayload(srv.Client(), "cover", srv.URL+"/song_1_cover.jpg", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "song_1_cover.jpg" {
		t.Fatalf("name: %#v", got["name"])
	}
	raw, err := base64.StdEncoding.DecodeString(got["data"].(string))
	if err != nil || string(raw) != "hello-file" {
		t.Fatalf("data: %#v err=%v", got["data"], err)
	}
}

func TestDownloadFilePayloadOversizeLimitReader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Content-Length: force LimitReader path.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 20)))
	}))
	defer srv.Close()

	_, err := downloadFilePayload(srv.Client(), "cover", srv.URL+"/big.bin", 10)
	if !IsErrFileTooLarge(err) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestPreflightFileSizesOversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "unexpected", http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	row := map[string]any{"cover": srv.URL + "/cover.jpg"}
	err := preflightFileSizes(srv.Client(), row, []string{"cover"}, 100)
	if !IsErrFileTooLarge(err) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestPreflightFileSizesOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "50")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	row := map[string]any{"cover": srv.URL + "/cover.jpg", "name": "x"}
	if err := preflightFileSizes(srv.Client(), row, []string{"cover"}, 100); err != nil {
		t.Fatal(err)
	}
}

func TestFileFieldURL(t *testing.T) {
	u, err := fileFieldURL(nil)
	if err != nil || u != "" {
		t.Fatalf("nil: %q %v", u, err)
	}
	u, err = fileFieldURL("https://x/y")
	if err != nil || u != "https://x/y" {
		t.Fatalf("url: %q %v", u, err)
	}
	if _, err := fileFieldURL(123); err == nil {
		t.Fatal("expected type error")
	}
	if _, err := fileFieldURL("/relative"); err == nil {
		t.Fatal("expected scheme error")
	}
}
