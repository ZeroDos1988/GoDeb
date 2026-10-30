package apphttps

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/acme"

	"godeb/plugins/domains"
)

// Hooks for tests.
var (
	obtainCert   = obtain
	publishA     = domains.Publish
	setChallenge = domains.SetACMEChallenge
	waitTXT      = waitAuthoritative
)

// directoryURL is Let's Encrypt; GODEB_ACME_DIRECTORY points elsewhere
// (staging, or a test server).
func directoryURL() string {
	if v := strings.TrimSpace(os.Getenv("GODEB_ACME_DIRECTORY")); strings.HasPrefix(v, "https://") {
		return v
	}
	return acme.LetsEncryptURL
}

// issued is the result of one ACME order.
type issued struct {
	AccountPEM string
	CertPEM    string
	KeyPEM     string
	NotAfter   time.Time
}

// obtain gets a certificate for host by DNS-01. accountPEM is the stored
// account key; a new one is created when it is empty.
func obtain(ctx context.Context, host, accountPEM string) (issued, error) {
	key, accountPEM, err := accountKey(accountPEM)
	if err != nil {
		return issued{}, err
	}
	client := &acme.Client{Key: key, DirectoryURL: directoryURL(), UserAgent: "godeb"}
	if _, err := client.Register(ctx, &acme.Account{}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return issued{}, fmt.Errorf("ACME-Konto: %w", err)
	}
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(host))
	if err != nil {
		return issued{}, fmt.Errorf("ACME-Auftrag: %w", err)
	}
	for _, u := range order.AuthzURLs {
		if err := authorize(ctx, client, host, u); err != nil {
			return issued{}, err
		}
	}
	if order, err = client.WaitOrder(ctx, order.URI); err != nil {
		return issued{}, fmt.Errorf("ACME-Auftrag: %w", err)
	}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return issued{}, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: host},
		DNSNames: []string{host},
	}, certKey)
	if err != nil {
		return issued{}, err
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return issued{}, fmt.Errorf("Zertifikat: %w", err)
	}
	if len(der) == 0 {
		return issued{}, errors.New("Zertifikat: leere Antwort")
	}
	leaf, err := x509.ParseCertificate(der[0])
	if err != nil {
		return issued{}, err
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return issued{}, fmt.Errorf("Zertifikat passt nicht zu %s: %w", host, err)
	}
	var chain []byte
	for _, d := range der {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: d})...)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(certKey)
	if err != nil {
		return issued{}, err
	}
	return issued{
		AccountPEM: accountPEM,
		CertPEM:    string(chain),
		KeyPEM:     string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		NotAfter:   leaf.NotAfter,
	}, nil
}

// authorize solves the dns-01 challenge of one authorization. The TXT
// record is removed again in every case.
func authorize(ctx context.Context, client *acme.Client, host, url string) error {
	z, err := client.GetAuthorization(ctx, url)
	if err != nil {
		return fmt.Errorf("ACME-Freigabe: %w", err)
	}
	if z.Status == acme.StatusValid {
		return nil
	}
	var chal *acme.Challenge
	for _, c := range z.Challenges {
		if c.Type == "dns-01" {
			chal = c
		}
	}
	if chal == nil {
		return errors.New("Let's Encrypt bietet keine DNS-Prüfung an")
	}
	value, err := client.DNS01ChallengeRecord(chal.Token)
	if err != nil {
		return err
	}
	zone, cleanup, err := setChallenge(ctx, host, value)
	if err != nil {
		return fmt.Errorf("TXT-Eintrag: %w", err)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanup(c)
	}()
	if err := waitTXT(ctx, zone, "_acme-challenge."+host, value); err != nil {
		return err
	}
	if _, err := client.Accept(ctx, chal); err != nil {
		return fmt.Errorf("ACME-Prüfung: %w", err)
	}
	if _, err := client.WaitAuthorization(ctx, z.URI); err != nil {
		return fmt.Errorf("ACME-Prüfung: %w", err)
	}
	return nil
}

func accountKey(stored string) (crypto.Signer, string, error) {
	if stored != "" {
		block, _ := pem.Decode([]byte(stored))
		if block == nil {
			return nil, "", errors.New("ACME-Kontoschlüssel ist beschädigt")
		}
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, "", err
		}
		signer, ok := k.(crypto.Signer)
		if !ok {
			return nil, "", errors.New("ACME-Kontoschlüssel ist beschädigt")
		}
		return signer, stored, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, "", err
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// waitAuthoritative waits until every name server of zone answers name with
// value, so Let's Encrypt sees the record on its first query.
func waitAuthoritative(ctx context.Context, zone, name, value string) error {
	servers, err := net.DefaultResolver.LookupNS(ctx, zone)
	if err != nil || len(servers) == 0 {
		return fmt.Errorf("Nameserver von %s nicht gefunden: %v", zone, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	for {
		pending := 0
		for _, ns := range servers {
			if !txtAt(ctx, ns.Host, name, value) {
				pending++
			}
		}
		if pending == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("TXT-Eintrag %s ist nach 5 Minuten bei %d Nameservern noch nicht sichtbar", name, pending)
		case <-time.After(10 * time.Second):
		}
	}
}

func txtAt(ctx context.Context, server, name, value string) bool {
	host := strings.TrimSuffix(server, ".")
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := net.Dialer{Timeout: 5 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(host, "53"))
	}}
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	txts, err := r.LookupTXT(c, strings.TrimSuffix(name, ".")+".")
	return err == nil && slices.Contains(txts, value)
}
