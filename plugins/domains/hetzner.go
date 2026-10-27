package domains

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	hetznerAPI   = "https://api.hetzner.cloud/v1"
	godebLabel   = "godeb"
	godebLabelOK = "1"
	httpTimeout  = 20 * time.Second
	maxPages     = 20
)

// httpDo is replaced in tests.
var httpDo = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: httpTimeout}
	return client.Do(req)
}

type hzError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e hzError) Error() string {
	if e.Message == "" {
		return "Hetzner-API-Fehler"
	}
	return "Hetzner: " + e.Message
}

var errZoneMissing = errors.New("nicht in diesem Hetzner-Projekt")

type hzZone struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type hzRecord struct {
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

type hzRRSet struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	TTL     *int              `json:"ttl"`
	Labels  map[string]string `json:"labels"`
	Records []hzRecord        `json:"records"`
}

type hzServer struct {
	ID        int64 `json:"id"`
	PublicNet struct {
		IPv4 *struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
	} `json:"public_net"`
}

func hzRequest(ctx context.Context, token, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, hetznerAPI+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpDo(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		var wrap struct {
			Error hzError `json:"error"`
		}
		_ = json.Unmarshal(data, &wrap)
		if wrap.Error.Message == "" {
			wrap.Error.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return data, resp.StatusCode, wrap.Error
	}
	return data, resp.StatusCode, nil
}

func validateToken(ctx context.Context, token string) error {
	if _, _, err := hzRequest(ctx, token, http.MethodGet, "/zones?per_page=1", nil); err != nil {
		return fmt.Errorf("Token ungültig oder ohne DNS-Recht: %w", err)
	}
	return nil
}

func findZone(ctx context.Context, token, name string) (hzZone, error) {
	data, code, err := hzRequest(ctx, token, http.MethodGet, "/zones/"+url.PathEscape(name), nil)
	if code == http.StatusNotFound {
		return hzZone{}, fmt.Errorf("Zone %s %w", name, errZoneMissing)
	}
	if err != nil {
		return hzZone{}, err
	}
	var wrap struct {
		Zone hzZone `json:"zone"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return hzZone{}, err
	}
	if wrap.Zone.Name == "" {
		wrap.Zone.Name = name
	}
	return wrap.Zone, nil
}

func rrsetPath(zone hzZone, name, typ string) string {
	return fmt.Sprintf("/zones/%s/rrsets/%s/%s", url.PathEscape(zone.Name), url.PathEscape(name), url.PathEscape(typ))
}

func getRRSet(ctx context.Context, token string, zone hzZone, name, typ string) (hzRRSet, bool, error) {
	data, code, err := hzRequest(ctx, token, http.MethodGet, rrsetPath(zone, name, typ), nil)
	if code == http.StatusNotFound {
		return hzRRSet{}, false, nil
	}
	if err != nil {
		return hzRRSet{}, false, err
	}
	var wrap struct {
		RRSet hzRRSet `json:"rrset"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return hzRRSet{}, false, err
	}
	return wrap.RRSet, true, nil
}

func createRRSet(ctx context.Context, token string, zone hzZone, name, typ, value string) error {
	body := map[string]any{
		"name":    name,
		"type":    typ,
		"ttl":     3600,
		"labels":  map[string]string{godebLabel: godebLabelOK},
		"records": []hzRecord{{Value: value, Comment: "GoDeb"}},
	}
	_, _, err := hzRequest(ctx, token, http.MethodPost, fmt.Sprintf("/zones/%s/rrsets", url.PathEscape(zone.Name)), body)
	return err
}

// setRecords replaces the values of an RRSet. PUT on an RRSet only changes
// its labels, so values go through the set_records action.
func setRecords(ctx context.Context, token string, zone hzZone, name, typ, value string) error {
	body := map[string]any{"records": []hzRecord{{Value: value, Comment: "GoDeb"}}}
	_, _, err := hzRequest(ctx, token, http.MethodPost, rrsetPath(zone, name, typ)+"/actions/set_records", body)
	return err
}

func isManaged(rr hzRRSet) bool {
	return rr.Labels != nil && rr.Labels[godebLabel] == godebLabelOK
}

func findServerID(ctx context.Context, token, publicIP string) (int64, error) {
	for page := 1; page <= maxPages; page++ {
		data, _, err := hzRequest(ctx, token, http.MethodGet, fmt.Sprintf("/servers?per_page=50&page=%d", page), nil)
		if err != nil {
			return 0, err
		}
		var wrap struct {
			Servers []hzServer `json:"servers"`
			Meta    struct {
				Pagination struct {
					NextPage *int `json:"next_page"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(data, &wrap); err != nil {
			return 0, err
		}
		for _, s := range wrap.Servers {
			if s.PublicNet.IPv4 != nil && s.PublicNet.IPv4.IP == publicIP {
				return s.ID, nil
			}
		}
		if wrap.Meta.Pagination.NextPage == nil {
			break
		}
	}
	return 0, fmt.Errorf("Kein Server mit öffentlicher IP %s in diesem Hetzner-Projekt", publicIP)
}

func changePTR(ctx context.Context, token string, serverID int64, ip, ptr string) error {
	body := map[string]any{
		"ip":      ip,
		"dns_ptr": strings.TrimSuffix(ptr, "."),
	}
	_, _, err := hzRequest(ctx, token, http.MethodPost, fmt.Sprintf("/servers/%d/actions/change_dns_ptr", serverID), body)
	return err
}

// tokenOK is a conservative allowlist for Hetzner API tokens.
func tokenOK(s string) bool {
	if len(s) < 20 || len(s) > 200 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
