package objsync

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	bobuild "github.com/bobuild/bobuild-client-go"

	"github.com/bobuild/bobuild-sync-objects/internal/config"
)

func newClient(app config.App) *bobuild.Client {
	c := bobuild.NewClient(app.Domain, app.APIKey)
	c.HTTPClient.Timeout = 120 * time.Second

	if app.Host == "" && !app.InsecureSkipVerify {
		return c
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if app.Host != "" {
		// Dial the ingress host/port over plain HTTP while keeping Domain for the
		// request Host / URL.
		c.UseTLS = false
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		host, port := app.Host, app.Port
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		}
	}
	if app.InsecureSkipVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
	c.HTTPClient.Transport = transport
	return c
}
