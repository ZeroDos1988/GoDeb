package domains

import (
	"bytes"
	"context"
	"encoding/json"
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
			wrap.Error.Message = strings.TrimSpace(string(data))
			if wrap.Error.Message == "" {
				wrap.Error.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
		}
		return data, resp.StatusCode, wrap.Error
	}
	return data, resp.StatusCode, nil
}

func listZones(ctx context.Context, token string) ([]hzZone, error) {
	data, _, err := hzRequest(ctx, token, http.MethodGet, "/zones?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Zones []hzZone `json:"zones"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, err
	}
	return wrap.Zones, nil
}

func findZone(ctx context.Context, token, name string) (hzZone, error) {
	zones, err := listZones(ctx, token)
	if err != nil {
		return hzZone{}, err
	}
	want := strings.TrimSuffix(strings.ToLower(name), ".")
	for _, z := range zones {
		if strings.EqualFold(strings.TrimSuffix(z.Name, "."), want) {
			return z, nil
		}
	}
	return hzZone{}, fmt.Errorf("Zone %s nicht in diesem Hetzner-Projekt", name)
}

func listRRSets(ctx context.Context, token string, zone hzZone) ([]hzRRSet, error) {
	path := fmt.Sprintf("/zones/%s/rrsets?per_page=100", url.PathEscape(zoneNameOrID(zone)))
	data, _, err := hzRequest(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		RRSets []hzRRSet `json:"rrsets"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, err
	}
	return wrap.RRSets, nil
}

func zoneNameOrID(z hzZone) string {
	if z.Name != "" {
		return z.Name
	}
	return fmt.Sprintf("%d", z.ID)
}

func getRRSet(ctx context.Context, token string, zone hzZone, name, typ string) (hzRRSet, bool, error) {
	path := fmt.Sprintf("/zones/%s/rrsets/%s/%s", url.PathEscape(zoneNameOrID(zone)), url.PathEscape(name), url.PathEscape(typ))
	data, code, err := hzRequest(ctx, token, http.MethodGet, path, nil)
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
		"name": name,
		"type": typ,
		"ttl":  3600,
		"labels": map[string]string{
			godebLabel: godebLabelOK,
		},
		"records": []hzRecord{{Value: value, Comment: "GoDeb"}},
	}
	path := fmt.Sprintf("/zones/%s/rrsets", url.PathEscape(zoneNameOrID(zone)))
	_, _, err := hzRequest(ctx, token, http.MethodPost, path, body)
	return err
}

func updateRRSet(ctx context.Context, token string, zone hzZone, name, typ, value string, labels map[string]string) error {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[godebLabel] = godebLabelOK
	ttl := 3600
	body := map[string]any{
		"ttl":     ttl,
		"labels":  labels,
		"records": []hzRecord{{Value: value, Comment: "GoDeb"}},
	}
	path := fmt.Sprintf("/zones/%s/rrsets/%s/%s", url.PathEscape(zoneNameOrID(zone)), url.PathEscape(name), url.PathEscape(typ))
	_, _, err := hzRequest(ctx, token, http.MethodPut, path, body)
	return err
}

func isManaged(rr hzRRSet) bool {
	return rr.Labels != nil && rr.Labels[godebLabel] == godebLabelOK
}

func findServerID(ctx context.Context, token, publicIP string) (int64, error) {
	data, _, err := hzRequest(ctx, token, http.MethodGet, "/servers?per_page=50", nil)
	if err != nil {
		return 0, err
	}
	var wrap struct {
		Servers []hzServer `json:"servers"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return 0, err
	}
	for _, s := range wrap.Servers {
		if s.PublicNet.IPv4 != nil && s.PublicNet.IPv4.IP == publicIP {
			return s.ID, nil
		}
	}
	return 0, fmt.Errorf("Kein Server mit öffentlicher IP %s in diesem Hetzner-Projekt", publicIP)
}

func changePTR(ctx context.Context, token string, serverID int64, ip, ptr string) error {
	body := map[string]any{
		"ip":      ip,
		"dns_ptr": strings.TrimSuffix(ptr, ".") + ".",
	}
	path := fmt.Sprintf("/servers/%d/actions/change_dns_ptr", serverID)
	_, _, err := hzRequest(ctx, token, http.MethodPost, path, body)
	return err
}

// tokenPattern: Hetzner tokens are long opaque strings; keep a conservative allowlist.
var tokenOK = func(s string) bool {
	s = strings.TrimSpace(s)
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
