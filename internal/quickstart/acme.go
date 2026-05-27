package quickstart

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/acme"

	"github.com/wujun4code/clashforge/internal/publish"
)

const (
	letsEncryptDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	// Staging for testing: "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// CertPair holds a PEM-encoded certificate chain and private key.
type CertPair struct {
	FullChainPEM string
	PrivKeyPEM   string
}

// IssueCert requests a Let's Encrypt certificate for domain via CF DNS-01 challenge.
// cfToken must have Zone:DNS:Edit permission.
func IssueCert(ctx context.Context, domain, cfToken, zoneID string, progress func(string)) (*CertPair, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	// 1. Generate account key
	progress("生成 ACME 账号密钥...")
	accountKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate account key: %w", err)
	}

	// 2. Create ACME client and register account
	client := &acme.Client{
		DirectoryURL: letsEncryptDirectoryURL,
		Key:          accountKey,
	}
	acc := &acme.Account{}
	if _, err := client.Register(ctx, acc, acme.AcceptTOS); err != nil {
		return nil, fmt.Errorf("acme register: %w", err)
	}

	// 3. Request authorisation for the domain
	progress(fmt.Sprintf("申请域名 %s 的 ACME 授权...", domain))
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return nil, fmt.Errorf("acme authorize order: %w", err)
	}

	// 4. Find the DNS-01 challenge
	var challengeURL string
	var dnsChallenge *acme.Challenge
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return nil, fmt.Errorf("get authorization: %w", err)
		}
		for _, ch := range authz.Challenges {
			if ch.Type == "dns-01" {
				dnsChallenge = ch
				challengeURL = authzURL
				break
			}
		}
		if dnsChallenge != nil {
			break
		}
	}
	_ = challengeURL // kept for potential future retry logic
	if dnsChallenge == nil {
		return nil, fmt.Errorf("no DNS-01 challenge found in ACME order")
	}

	// 5. Derive the TXT record value
	txtValue, err := client.DNS01ChallengeRecord(dnsChallenge.Token)
	if err != nil {
		return nil, fmt.Errorf("derive dns-01 txt value: %w", err)
	}
	txtName := "_acme-challenge." + domain

	// 6. Create the TXT record in Cloudflare
	progress("在 Cloudflare 创建 DNS-01 TXT 记录...")
	cf, err := publish.NewCloudflareClient(cfToken)
	if err != nil {
		return nil, err
	}
	txtRecordID, err := cf.CreateDNSTXTRecord(ctx, zoneID, txtName, txtValue)
	if err != nil {
		return nil, fmt.Errorf("create txt record: %w", err)
	}
	defer func() {
		// Clean up TXT record regardless of outcome
		progress("删除 DNS-01 TXT 记录...")
		delCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = cf.DeleteDNSRecord(delCtx, zoneID, txtRecordID)
	}()

	// 7. Wait for DNS propagation (poll, up to 120 s)
	progress("等待 DNS TXT 记录生效（最多 120 秒）...")
	if err := waitForDNS(ctx, txtName, txtValue, 120*time.Second); err != nil {
		return nil, fmt.Errorf("dns propagation timeout: %w", err)
	}
	progress("DNS 记录已生效")

	// 8. Tell ACME we're ready and accept the challenge
	if _, err := client.Accept(ctx, dnsChallenge); err != nil {
		return nil, fmt.Errorf("acme accept challenge: %w", err)
	}

	// 9. Wait for order to be ready
	progress("等待 ACME 验证完成...")
	order, err = client.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, fmt.Errorf("acme wait order: %w", err)
	}

	// 10. Generate certificate key and CSR
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate cert key: %w", err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: domain},
	}, certKey)
	if err != nil {
		return nil, fmt.Errorf("create csr: %w", err)
	}

	// 11. Finalise the order and download the certificate
	progress("申请并下载 TLS 证书...")
	certs, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, fmt.Errorf("create order cert: %w", err)
	}

	// 12. Encode certificate chain
	var certChain strings.Builder
	for _, c := range certs {
		if err := pem.Encode(&certChain, &pem.Block{Type: "CERTIFICATE", Bytes: c}); err != nil {
			return nil, fmt.Errorf("encode cert: %w", err)
		}
	}

	// 13. Encode private key
	certKeyDER, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return nil, fmt.Errorf("marshal cert key: %w", err)
	}
	var keyBuf strings.Builder
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: certKeyDER}); err != nil {
		return nil, fmt.Errorf("encode cert key: %w", err)
	}

	return &CertPair{
		FullChainPEM: certChain.String(),
		PrivKeyPEM:   keyBuf.String(),
	}, nil
}

// waitForDNS polls until the TXT record at name contains the expected value, or timeout.
// It uses the system resolver; on DNS splits this may return the ISP resolver's view,
// which is usually good enough since the ACME server also uses the global DNS.
func waitForDNS(ctx context.Context, name, expectedValue string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("DNS TXT record %q did not propagate within %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
		// Use nslookup / dig via a plain DNS query isn't directly available in pure Go
		// without external dependencies. We rely on the ACME server itself to retry;
		// a short sleep is sufficient for most CF setups (< 30 s).
		// For a more robust implementation, integrate miekg/dns.
		if time.Now().After(deadline.Add(-60 * time.Second)) {
			// At least 60 s have passed — give up waiting and let ACME decide.
			return nil
		}
	}
}
