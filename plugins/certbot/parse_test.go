package certbot

import (
	"testing"
	"time"

	"godeb/backend/metrics"
)

func TestParseOpenSSL(t *testing.T) {
	subject, notBefore, notAfter := ParseOpenSSL("subject=CN = example.com\nnotBefore=Jan  1 00:00:00 2026 GMT\nnotAfter=Apr  1 00:00:00 2026 GMT\n")
	if subject != "CN = example.com" || notBefore == "" || notAfter == "" {
		t.Fatalf("got %q %q %q", subject, notBefore, notAfter)
	}
}

func TestExpiryFinding(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	if _, ok := ExpiryFinding("a", "Jan  1 00:00:00 2027 GMT", now); ok {
		t.Fatal("70 days left is fine")
	}
	if f, ok := ExpiryFinding("a", "Nov  1 00:00:00 2026 GMT", now); !ok || f.Status != metrics.StatusWarning {
		t.Fatalf("9 days = %+v", f)
	}
	if f, ok := ExpiryFinding("a", "Oct 24 00:00:00 2026 GMT", now); !ok || f.Status != metrics.StatusCritical {
		t.Fatalf("12 hours = %+v", f)
	}
}
