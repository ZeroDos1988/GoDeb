package certbot

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"godeb/backend/core"
)

func init() {
	core.Register("certbot", Mount)
}

const liveRoot = "/etc/letsencrypt/live"

var lineageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Mount registers the Certbot route.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/certbot/certificates", handleCertificates)
}

type response struct {
	Path         string        `json:"path"`
	Command      []string      `json:"command"`
	Mocked       bool          `json:"mocked"`
	Detail       string        `json:"detail,omitempty"`
	Certificates []Certificate `json:"certificates"`
}

func handleCertificates(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, collect(r.Context()))
}

func collect(ctx context.Context) response {
	resp := response{
		Path:         liveRoot,
		Command:      []string{"openssl", "x509", "-in", liveRoot + "/<name>/cert.pem", "-noout", "-subject", "-startdate", "-enddate"},
		Certificates: []Certificate{},
	}
	names, err := lineageNames()
	if err != nil {
		resp.Mocked = true
		resp.Detail = err.Error()
		resp.Certificates = sampleCertificates()
		return resp
	}
	var notes []string
	for _, name := range names {
		cert := Certificate{Name: name, Path: filepath.Join(liveRoot, name)}
		subject, notBefore, notAfter, detail := inspect(ctx, filepath.Join(liveRoot, name, "cert.pem"))
		cert.Subject = subject
		cert.NotBefore = notBefore
		cert.NotAfter = notAfter
		if detail != "" {
			notes = append(notes, name+": "+detail)
		}
		resp.Certificates = append(resp.Certificates, cert)
	}
	if len(notes) > 0 {
		resp.Detail = strings.Join(notes, "; ")
	}
	return resp
}

func lineageNames() ([]string, error) {
	entries, err := os.ReadDir(liveRoot)
	if err != nil {
		if os.IsPermission(err) && os.Geteuid() != 0 {
			res := core.Run(context.Background(), "sudo", "-n", "--", "ls", "-1", liveRoot)
			if res.Err == nil {
				return filterNames(strings.Split(res.Stdout, "\n")), nil
			}
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return filterNames(names), nil
}

func filterNames(names []string) []string {
	out := []string{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if lineageName.MatchString(name) {
			out = append(out, name)
		}
	}
	return out
}

func inspect(ctx context.Context, certPath string) (subject, notBefore, notAfter, detail string) {
	clean := filepath.Clean(certPath)
	if !strings.HasPrefix(clean, liveRoot+string(os.PathSeparator)) {
		return "", "", "", "Pfad liegt außerhalb von /etc/letsencrypt/live"
	}
	res := core.Run(ctx, "openssl", "x509", "-in", clean, "-noout", "-subject", "-startdate", "-enddate")
	if res.Err != nil && os.Geteuid() != 0 {
		res = core.RunAdmin(ctx, "openssl", "x509", "-in", clean, "-noout", "-subject", "-startdate", "-enddate")
	}
	if res.Err != nil {
		return "", "", "", res.Detail()
	}
	subject, notBefore, notAfter = ParseOpenSSL(res.Stdout)
	return subject, notBefore, notAfter, ""
}
