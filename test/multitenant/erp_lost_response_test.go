package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

var errERPReplyLost = errors.New("synthetic response lost after successful server response")

// The backend receives a real HTTP request and returns a full successful JSON
// response. Only this test client's transport withholds that response, proving
// an uncertain outcome without adding fault switches to the application.
type erpLostResponseTransport struct {
	base      http.RoundTripper
	requests  int
	committed bool
}

func (p *erpLostResponseTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.requests++
	if p.requests != 1 {
		return nil, errors.New("unexpected automatic retry by test client")
	}
	res, err := p.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Code int `json:"code"`
	}
	if len(raw) > limit || res.StatusCode != http.StatusOK || json.Unmarshal(raw, &envelope) != nil || envelope.Code != 200 {
		return nil, fmt.Errorf("cannot simulate loss: backend HTTP=%d business=%d", res.StatusCode, envelope.Code)
	}
	p.committed = true
	return nil, errERPReplyLost
}

func loseERPResponseAfterCommit(t *testing.T, baseURL, token, method, path string, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	transport := &erpLostResponseTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if res != nil {
		res.Body.Close()
	}
	if !errors.Is(err, errERPReplyLost) || !transport.committed || transport.requests != 1 {
		t.Fatal("request must commit once, then fail at this client's response boundary")
	}
}

func TestERPBusinessFlowLostResponses(t *testing.T) {
	runERPBusinessFlow(t, true)
}
