package executor

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The accelerated host was returned by the authenticated image API in the 2026-11-12 smoke test.
var qwenResultHost = regexp.MustCompile(`^(dashscope-result[a-z0-9-]*\.oss-[a-z0-9-]+\.aliyuncs\.com|dashscope-a717\.oss-accelerate\.aliyuncs\.com)$`)

func ValidQwenImageURL(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && u.Fragment == "" && qwenResultHost.MatchString(u.Host)
}

func publicQwenAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32", "2002::/16"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return true
}

func NewQwenImageClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || !qwenResultHost.MatchString(host) {
			return nil, errors.New("invalid image address")
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("image DNS unavailable")
		}
		for _, ip := range ips {
			if !publicQwenAddress(ip) {
				return nil, errors.New("private image address rejected")
			}
		}
		var conn net.Conn
		for _, ip := range ips {
			conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errors.New("image connection unavailable")
	}
	return &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("image redirect rejected") }}
}

func DownloadQwenImage(ctx context.Context, client *http.Client, raw string, maxBytes int64) ([]byte, error) {
	if !ValidQwenImageURL(raw) {
		return nil, errors.New("供应商图片地址或大小限制不合法")
	}
	return downloadImage(ctx, client, raw, maxBytes)
}

func downloadImage(ctx context.Context, client *http.Client, raw string, maxBytes int64) ([]byte, error) {
	return downloadImageWithBearer(ctx, client, raw, maxBytes, "")
}

func downloadImageWithBearer(ctx context.Context, client *http.Client, raw string, maxBytes int64, apiKey string) ([]byte, error) {
	if maxBytes < 1 || maxBytes > 20<<20 {
		return nil, errors.New("图片大小限制不合法")
	}
	if client == nil {
		return nil, errors.New("图片下载客户端未配置")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("供应商图片请求无效")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("供应商图片暂时无法下载，保留原结果等待归档")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxBytes || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		return nil, errors.New("供应商图片响应无效或地址过期，请核对原调用")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errors.New("供应商图片不完整或过大")
	}
	return data, nil
}
