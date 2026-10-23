package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"godeb/backend/firstrun"
)

// tlsListeners runs the one HTTPS listener GoDeb has besides loopback and
// wg0: the public setup listener while the first-run setup is open, later
// the LAN listener if the admin chose access from the home network.
type tlsListeners struct {
	mu      sync.Mutex
	handler http.Handler
	servers map[string]*http.Server
	wake    chan struct{}
}

func newTLSListeners(h http.Handler) *tlsListeners {
	t := &tlsListeners{handler: h, servers: map[string]*http.Server{}, wake: make(chan struct{}, 1)}
	firstrun.OnChange(func() {
		select {
		case t.wake <- struct{}{}:
		default:
		}
	})
	return t
}

func (t *tlsListeners) desired() string {
	if firstrun.Required() {
		return firstrun.SetupAddr()
	}
	return firstrun.LoadAccess().Addr()
}

func (t *tlsListeners) sync() {
	want := t.desired()
	t.mu.Lock()
	defer t.mu.Unlock()
	for addr, srv := range t.servers {
		if addr == want {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		delete(t.servers, addr)
		slog.Info("HTTPS geschlossen", "adresse", addr)
	}
	if want == "" || t.servers[want] != nil {
		return
	}
	cert, fp, err := firstrun.EnsureCert()
	if err != nil {
		slog.Error("Zertifikat nicht verfügbar", "fehler", err)
		return
	}
	ln, err := net.Listen("tcp", want)
	if err != nil {
		slog.Warn("HTTPS noch nicht möglich", "adresse", want, "fehler", err)
		return
	}
	srv := &http.Server{
		Handler:           t.handler,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	t.servers[want] = srv
	go func() {
		if err := srv.Serve(tls.NewListener(ln, srv.TLSConfig)); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTPS-Server beendet", "adresse", want, "fehler", err)
		}
	}()
	if firstrun.Required() {
		slog.Warn("Ersteinrichtung offen, Link mit Token: sudo godeb setup-url", "url", "https://"+want, "zertifikat_sha256", fp)
	} else {
		slog.Info("GoDeb lauscht im Heimnetz", "url", "https://"+want)
	}
}

// run keeps the listener in line with the setup state and access mode. The
// poll catches changes made by the CLI, which runs in another process.
func (t *tlsListeners) run(ctx context.Context) {
	for {
		t.sync()
		select {
		case <-ctx.Done():
			return
		case <-t.wake:
		case <-time.After(3 * time.Second):
		}
	}
}
