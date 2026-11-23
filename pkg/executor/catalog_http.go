package executor

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

var catalogModelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/@+-]{0,127}$`)
var catalogHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?\.[a-z]{2,63}$`)
var catalogPath = regexp.MustCompile(`^(/[a-zA-Z0-9_~.-]+)*$`)

func ValidCatalogModelID(id string) bool { return len(id) <= 80 && catalogModelID.MatchString(id) }

// Base URLs are configuration, never a URL supplied by a creator or a workflow.
func ValidCatalogEndpoint(raw string) bool {
	if len(raw) > 200 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !publicCatalogURL(u) || u.RawQuery != "" || u.RawPath != "" || !catalogPath.MatchString(u.Path) || strings.HasSuffix(u.Path, "/") {
		return false
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func publicCatalogURL(u *url.URL) bool {
	if u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.Host != strings.ToLower(u.Host) {
		return false
	}
	host := u.Hostname()
	if !catalogHost.MatchString(host) || strings.Contains(host, "..") {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".home", ".lan"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return true
}

func ValidCatalogImageURL(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && publicCatalogURL(u)
}

// Resolve once and dial the checked IP. No proxy, redirects or ambient cookies.
func NewCatalogHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout, MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || !publicCatalogURL(&url.URL{Scheme: "https", Host: host}) {
			return nil, errors.New("invalid public HTTPS address")
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("provider DNS unavailable")
		}
		if !catalogPublicIPs(ips) {
			return nil, errors.New("private provider address rejected")
		}
		for _, ip := range ips {
			conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
		}
		return nil, errors.New("provider connection unavailable")
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirect rejected") }}
}

func catalogPublicIPs(ips []netip.Addr) bool {
	for _, ip := range ips {
		if !publicQwenAddress(ip) || netip.MustParsePrefix("240.0.0.0/4").Contains(ip.Unmap()) || (ip.Is6() && !ip.Is4In6() && !netip.MustParsePrefix("2000::/3").Contains(ip)) {
			return false
		}
	}
	return len(ips) > 0
}

const catalogInlineImageBytes = 8 << 20

func decodeCatalogInline(raw string, maxBytes int64) ([]byte, error) {
	for _, media := range []string{"image/png", "image/jpeg", "image/webp"} {
		prefix := "data:" + media + ";base64,"
		if !strings.HasPrefix(raw, prefix) {
			continue
		}
		encoded := raw[len(prefix):]
		limit := min(maxBytes, catalogInlineImageBytes)
		if limit < 1 || len(encoded) > base64.StdEncoding.EncodedLen(int(limit)) {
			break
		}
		data, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err == nil && int64(len(data)) <= limit && http.DetectContentType(data) == media {
			return data, nil
		}
	}
	return nil, errors.New("invalid or oversized inline image")
}

func DownloadCatalogImage(ctx context.Context, client *http.Client, raw string, maxBytes int64) ([]byte, error) {
	if strings.HasPrefix(raw, "data:") {
		return decodeCatalogInline(raw, maxBytes)
	}
	if !ValidCatalogImageURL(raw) {
		return nil, errors.New("invalid provider result URL")
	}
	// Reuse the bounded reader, but keep the legacy Qwen host policy unchanged.
	return downloadImage(ctx, client, raw, maxBytes)
}

const BBRootImageAPIEndpoint = "https://bian.bbroot.com/v1"

// Only the verified private BBRoot route needs credentials; signed and CDN URLs never do.
func IsBBRootPrivateImageURL(raw string) bool {
	if !ValidCatalogImageURL(raw) {
		return false
	}
	u, _ := url.Parse(raw)
	return u.Host == "bian.bbroot.com" && u.RawQuery == "" && !u.ForceQuery &&
		u.RawPath == "" && strings.HasPrefix(u.Path, "/images/") && path.Clean(u.Path) == u.Path
}

func DownloadBBRootPrivateImage(ctx context.Context, client *http.Client, raw string, maxBytes int64, apiKey string) ([]byte, error) {
	if client == nil || !IsBBRootPrivateImageURL(raw) || len(apiKey) < 8 || len(apiKey) > 512 || strings.ContainsAny(apiKey, "\r\n \t") || strings.HasPrefix(apiKey, "sess-") {
		return nil, errors.New("必安私有图片地址或 API 凭据无效")
	}
	c := *client
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("provider redirect rejected") }
	return downloadImageWithBearer(ctx, &c, raw, maxBytes, apiKey)
}
