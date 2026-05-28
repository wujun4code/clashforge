package quickstart

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// ExistingCheckResult is returned by CheckExistingDeployment.
type ExistingCheckResult struct {
	Reusable   bool   // true → skip deployment, go straight to import
	DaysLeft   int    // TLS cert days remaining (0 if not reusable)
	SkipReason string // human-readable explanation when Reusable=false
}

// CheckExistingDeployment checks whether a gost SOCKS5+TLS service is already
// running at domain:443 and points to vpsIP.
//
//  1. DNS: domain must resolve to vpsIP
//  2. TCP: port 443 must be reachable within 5 s
//  3. TLS: cert must be valid and expire >14 days from now
//
// Returns Reusable=true only when all three checks pass.
func CheckExistingDeployment(ctx context.Context, domain, vpsIP string, out EventWriter) ExistingCheckResult {
	emit(out, PhaseExistingCheck, "dns_check", StatusRunning,
		fmt.Sprintf("解析域名 %s ...", domain))

	addrs, err := net.DefaultResolver.LookupHost(ctx, domain)
	if err != nil {
		msg := fmt.Sprintf("域名解析失败：%v", err)
		emit(out, PhaseExistingCheck, "dns_check", StatusInfo, msg)
		return ExistingCheckResult{SkipReason: msg}
	}

	found := false
	for _, a := range addrs {
		if a == vpsIP {
			found = true
			break
		}
	}
	if !found {
		msg := fmt.Sprintf("DNS 指向 %v，与 VPS IP %s 不匹配，将重新部署", addrs, vpsIP)
		emit(out, PhaseExistingCheck, "dns_check", StatusInfo, msg)
		return ExistingCheckResult{SkipReason: msg}
	}
	emit(out, PhaseExistingCheck, "dns_check", StatusOK,
		fmt.Sprintf("DNS 正确：%s → %s", domain, vpsIP))

	// ── TCP reachability ─────────────────────────────────────────────────────
	emit(out, PhaseExistingCheck, "tcp_check", StatusRunning,
		fmt.Sprintf("检测 %s:443 TCP 连通性...", domain))

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tcpConn, err := dialer.DialContext(ctx, "tcp", domain+":443")
	if err != nil {
		msg := fmt.Sprintf("TCP 443 不可达：%v", err)
		emit(out, PhaseExistingCheck, "tcp_check", StatusInfo, msg)
		return ExistingCheckResult{SkipReason: msg}
	}
	tcpConn.Close()
	emit(out, PhaseExistingCheck, "tcp_check", StatusOK, "TCP 443 可达")

	// ── TLS cert validity ────────────────────────────────────────────────────
	emit(out, PhaseExistingCheck, "tls_check", StatusRunning, "检测 TLS 证书有效期...")

	tlsCfg := &tls.Config{ServerName: domain, InsecureSkipVerify: false}
	tlsDialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 8 * time.Second},
		Config:    tlsCfg,
	}
	tlsConn, err := tlsDialer.DialContext(ctx, "tcp", domain+":443")
	if err != nil {
		msg := fmt.Sprintf("TLS 握手失败：%v", err)
		emit(out, PhaseExistingCheck, "tls_check", StatusInfo, msg)
		return ExistingCheckResult{SkipReason: msg}
	}
	defer tlsConn.Close()

	certs := tlsConn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		msg := "TLS 握手成功但未返回证书"
		emit(out, PhaseExistingCheck, "tls_check", StatusInfo, msg)
		return ExistingCheckResult{SkipReason: msg}
	}

	expiry := certs[0].NotAfter
	daysLeft := int(time.Until(expiry).Hours() / 24)
	if daysLeft <= 14 {
		msg := fmt.Sprintf("TLS 证书将在 %d 天后过期，需重新申请证书", daysLeft)
		emit(out, PhaseExistingCheck, "tls_check", StatusInfo, msg)
		return ExistingCheckResult{SkipReason: msg}
	}

	emit(out, PhaseExistingCheck, "tls_check", StatusOK,
		fmt.Sprintf("TLS 证书有效，剩余 %d 天（到期 %s）", daysLeft, expiry.Format("2006-01-02")))

	emit(out, PhaseExistingCheck, "result", StatusOK,
		fmt.Sprintf("节点 %s 已存在且可用，跳过部署，直接导入订阅", domain))

	return ExistingCheckResult{Reusable: true, DaysLeft: daysLeft}
}
