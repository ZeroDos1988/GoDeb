package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"godeb/plugins/apphttps"
)

// appTLS serves HTTPS with the Let's Encrypt certificate of the apphttps
// plugin on the wg0 address only. The agent writes listen file, key and
// certificate into the state directory; this process follows them.
type appTLS struct {
	handler http.Handler
	srv     *http.Server
	addr    string
	certs   certFiles
}

func (a *appTLS) run(ctx context.Context) {
	for {
		a.sync()
		select {
		case <-ctx.Done():
			a.stop()
			return
		case <-time.After(15 * time.Second):
		}
	}
}

func (a *appTLS) want() string {
	l := apphttps.LoadListen()
	if !l.Enabled {
		return ""
	}
	addr, err := listenSpec{Iface: "wg0", Port: strconv.Itoa(l.Port)}.resolve()
	if err != nil {
		return ""
	}
	return addr
}

func (a *appTLS) sync() {
	want := a.want()
	if want == a.addr {
		return
	}
	a.stop()
	if want == "" {
		return
	}
	if _, err := a.certs.get(nil); err != nil {
		slog.Warn("App-Zugang: Zertifikat nicht lesbar", "fehler", err)
		return
	}
	ln, err := net.Listen("tcp", want)
	if err != nil {
		slog.Warn("App-Zugang: HTTPS noch nicht möglich", "adresse", want, "fehler", err)
		return
	}
	srv := &http.Server{
		Handler:           a.handler,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{GetCertificate: a.certs.get, MinVersion: tls.VersionTLS12},
	}
	a.srv, a.addr = srv, want
	go func() {
		if err := srv.Serve(tls.NewListener(ln, srv.TLSConfig)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("App-Zugang: HTTPS-Server beendet", "adresse", want, "fehler", err)
		}
	}()
	slog.Info("GoDeb lauscht per HTTPS im Tunnel", "adresse", want, "name", apphttps.LoadListen().Host)
}

func (a *appTLS) stop() {
	if a.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = a.srv.Shutdown(ctx)
	cancel()
	slog.Info("App-Zugang: HTTPS geschlossen", "adresse", a.addr)
	a.srv, a.addr = nil, ""
}

// certFiles reloads the key pair when the certificate file changes, so a
// renewal takes effect without a restart.
type certFiles struct {
	mu    sync.Mutex
	mtime time.Time
	cert  *tls.Certificate
}

func (c *certFiles) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := os.Stat(apphttps.CertPath())
	if err != nil {
		if c.cert != nil {
			return c.cert, nil
		}
		return nil, err
	}
	if c.cert != nil && info.ModTime().Equal(c.mtime) {
		return c.cert, nil
	}
	cert, err := tls.LoadX509KeyPair(apphttps.CertPath(), apphttps.KeyPath())
	if err != nil {
		if c.cert != nil {
			return c.cert, nil
		}
		return nil, err
	}
	c.cert, c.mtime = &cert, info.ModTime()
	return c.cert, nil
}
