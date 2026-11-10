package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"godeb/backend/doctor"
)

// runDoctor prints the read-only health check of this host.
func runDoctor(stdout, stderr io.Writer) int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(stderr, "godeb doctor muss als root laufen: sudo godeb doctor")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fmt.Fprintf(stdout, "GoDeb Doctor – %s – %s\n\n", hostname(), time.Now().Format("2006-01-02 15:04"))
	if fails, _ := doctor.Write(stdout, doctor.Check(ctx, doctorEnv())); fails > 0 {
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

// runDoctorText prints the diagnosis report as on the overview; with
// --redact public addresses, host and domain names are replaced.
func runDoctorText(args []string, stdout, stderr io.Writer) int {
	redact := false
	for _, a := range args {
		if a != "--redact" {
			fmt.Fprintln(stderr, "Aufruf: godeb doctor --report [--redact]")
			return 2
		}
		redact = true
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(stderr, "godeb doctor muss als root laufen: sudo godeb doctor --report")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out := renderDoctorText(runFullDoctorReport(ctx), redact, time.Now())
	fmt.Fprint(stdout, out.Text)
	return 0
}
