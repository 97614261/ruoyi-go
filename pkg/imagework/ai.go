package imagework

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var aiSlots = make(chan struct{}, 1)

// The optional self-hosted CPU service receives bytes, never private URLs or credentials.
// Restrict it to a literal loopback address; redirects must not turn it into an SSRF proxy.
func ValidAIURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

func AI(ctx context.Context, base string, raw []byte, operation string, scale int, maxBytes int64) ([]byte, error) {
	if !ValidAIURL(base) {
		return nil, errors.New("本地 AI 图片服务未配置，未执行 AI 处理")
	}
	if operation != "removeBackground" && operation != "upscale" {
		return nil, errors.New("不支持的 AI 处理方式")
	}
	select {
	case aiSlots <- struct{}{}:
		defer func() { <-aiSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/process?operation="+operation+"&scale="+strconv.Itoa(scale), bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("无法连接本地 AI 图片服务")
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	client := &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 1, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("本地 AI 图片服务未响应，请确认已启动；没有调用付费模型")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" {
		return nil, errors.New("本地 AI 图片处理失败或模型未准备好，请检查图片服务状态")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errors.New("AI 结果读取失败或文件过大")
	}
	return data, nil
}
