package objsync

import (
	"context"
	"net"
	"net/http"
	"time"

	bobuild "github.com/bobuild/bobuild-client-go"

	"github.com/bobuild/bobuild-sync-objects/internal/config"
)

func newClient(app config.App) *bobuild.Client {
	c := bobuild.NewClient(app.Domain, app.APIKey)
	c.HTTPClient.Timeout = 120 * time.Second

	if app.Host == "" {
		return c
	}

	// Dial the ingress host/port over plain HTTP while keeping Domain for the
	// request Host / URL.
	c.UseTLS = false
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	host, port := app.Host, app.Port
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	c.HTTPClient.Transport = transport
	return c
}
