package objsync

import (
	"net/http"
	"testing"

	"github.com/bobuild/bobuild-sync-objects/internal/config"
)

func TestNewClientInsecureSkipVerify(t *testing.T) {
	c := newClient(config.App{
		Domain:             "example.com",
		APIKey:             "k",
		InsecureSkipVerify: true,
	})
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify transport, got %#v", c.HTTPClient.Transport)
	}
}

func TestNewClientDefaultKeepsTransport(t *testing.T) {
	c := newClient(config.App{
		Domain: "example.com",
		APIKey: "k",
	})
	if c.HTTPClient.Transport != nil {
		t.Fatalf("expected default transport unset, got %#v", c.HTTPClient.Transport)
	}
}
