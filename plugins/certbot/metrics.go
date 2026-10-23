package certbot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"godeb/backend/metrics"
)

func init() {
	metrics.RegisterCheck("certbot", expiryCheck)
}

var (
	expiryMu   sync.Mutex
	expiryAt   time.Time
	expiryLast []metrics.Finding
)

// expiryCheck warns 14 days and alarms 3 days before a certificate expires.
// openssl runs at most once an hour.
func expiryCheck(ctx context.Context) []metrics.Finding {
	expiryMu.Lock()
	defer expiryMu.Unlock()
	if time.Since(expiryAt) < time.Hour {
		return expiryLast
	}
	expiryAt = time.Now()
	resp := collect(ctx)
	expiryLast = nil
	if resp.Mocked {
		return nil
	}
	for _, c := range resp.Certificates {
		if f, ok := ExpiryFinding(c.Name, c.NotAfter, time.Now()); ok {
			expiryLast = append(expiryLast, f)
		}
	}
	return expiryLast
}

// ExpiryFinding rates one certificate by its openssl notAfter date.
func ExpiryFinding(name, notAfter string, now time.Time) (metrics.Finding, bool) {
	end, err := time.Parse("Jan _2 15:04:05 2006 MST", notAfter)
	if err != nil {
		return metrics.Finding{}, false
	}
	left := end.Sub(now)
	f := metrics.Finding{ID: "cert:" + name, Detail: fmt.Sprintf("gültig bis %s (certbot renew prüfen)", end.Format("02.01.2006"))}
	switch {
	case left < 3*24*time.Hour:
		f.Status, f.Title = metrics.StatusCritical, "Zertifikat läuft ab: "+name
	case left < 14*24*time.Hour:
		f.Status, f.Title = metrics.StatusWarning, "Zertifikat läuft bald ab: "+name
	default:
		return metrics.Finding{}, false
	}
	return f, true
}
