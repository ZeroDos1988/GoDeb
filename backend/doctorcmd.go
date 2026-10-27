package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"godeb/backend/doctor"
	"godeb/plugins/domains"
)

// runDoctor prints the read-only health check of this host.
func runDoctor(stdout, stderr io.Writer) int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(stderr, "godeb doctor muss als root laufen: sudo godeb doctor")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	env := doctor.Default(version)
	env.DNS = func(ctx context.Context) ([]doctor.DNSRecord, error) {
		recs, err := domains.CheckAll(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]doctor.DNSRecord, 0, len(recs))
		for _, r := range recs {
			out = append(out, doctor.DNSRecord{Name: r.Name, Type: r.Type, Status: r.Status, Expected: r.ZoneLine, Found: r.Found})
		}
		return out, nil
	}
	fmt.Fprintf(stdout, "GoDeb Doctor – %s – %s\n\n", hostname(), time.Now().Format("2006-01-02 15:04"))
	if fails, _ := doctor.Write(stdout, doctor.Check(ctx, env)); fails > 0 {
		return 1
	}
	return 0
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "?"
	}
	return h
}
