// Command erpbench previews or measures bounded read-only local ERP requests.
// It never creates fixtures, logs in, changes a password, alters indexes or
// accepts a password/token on the command line. See docs/ERP_CAPACITY.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/test/erpload"
)

type plan struct {
	Execute   bool            `json:"execute"`
	Config    string          `json:"config"`
	Base      string          `json:"base"`
	Path      string          `json:"path"`
	TokenFile string          `json:"-"`
	Options   erpload.Options `json:"options"`
}

func parse(args []string) (plan, error) {
	var p plan
	f := flag.NewFlagSet("erpbench", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.BoolVar(&p.Execute, "execute", false, "execute local read load; otherwise preview")
	f.StringVar(&p.Config, "config", "configs/multitenant.local.yml", "dedicated local config")
	f.StringVar(&p.Base, "url", "http://127.0.0.1:8081", "only the dedicated loopback backend")
	f.StringVar(&p.Path, "path", "/erp/order/list?pageSize=20", "read-only ERP path")
	f.StringVar(&p.TokenFile, "token-file", "", "private JSON file containing token, read only on execute")
	f.IntVar(&p.Options.Concurrency, "c", 8, "in-flight requests")
	f.DurationVar(&p.Options.Duration, "duration", 15*time.Second, "scheduling duration")
	f.Int64Var(&p.Options.MaxRequests, "n", 10000, "upper request bound")
	f.DurationVar(&p.Options.RequestTimeout, "timeout", 10*time.Second, "individual timeout")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return p, fmt.Errorf("invalid arguments")
	}
	if p.Base != "http://127.0.0.1:8081" {
		return p, fmt.Errorf("only dedicated loopback backend is allowed")
	}
	u, e := url.ParseRequestURI(p.Path)
	if e != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/erp/") || u.Fragment != "" || strings.Contains(u.Path, "..") || strings.ContainsAny(u.Path, "\\\r\n") {
		return p, fmt.Errorf("expected an ERP relative request path")
	}
	o := p.Options
	if o.Concurrency < 1 || o.Concurrency > 256 || o.Duration <= 0 || o.Duration > 2*time.Minute || o.MaxRequests < 1 || o.MaxRequests > 250000 || o.RequestTimeout <= 0 || o.RequestTimeout > 30*time.Second {
		return p, fmt.Errorf("invalid load bounds")
	}
	return p, nil
}
func localConfig(cfg *config.Config) bool {
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		return false
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (h != "127.0.0.1" && h != "localhost") {
		return false
	}
	h, _, e = net.SplitHostPort(cfg.Redis.Addr)
	return e == nil && (h == "127.0.0.1" || h == "localhost") && cfg.Redis.DB == 13
}
func run(args []string, out io.Writer) error {
	p, e := parse(args)
	if e != nil {
		return e
	}
	if !p.Execute {
		return json.NewEncoder(out).Encode(p)
	}
	if p.TokenFile == "" {
		return fmt.Errorf("execute requires a private token file; no automatic login")
	}
	cfg, e := config.Load(p.Config)
	if e != nil || !localConfig(cfg) {
		return fmt.Errorf("dedicated local configuration required")
	}
	f, e := os.Open(p.TokenFile)
	if e != nil {
		return fmt.Errorf("private token file unavailable")
	}
	defer f.Close()
	var secret struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&secret) != nil || secret.Token == "" || strings.ContainsAny(secret.Token, "\r\n") {
		return fmt.Errorf("invalid private token record")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = p.Options.Concurrency * 2
	tr.MaxIdleConnsPerHost = p.Options.Concurrency * 2
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: p.Options.RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirects are not followed") }}
	target := erpload.Target{Name: "erp_read", Check: erpload.JSON200, Request: func(ctx context.Context, _ int64) (*http.Request, error) {
		r, e := http.NewRequestWithContext(ctx, "GET", p.Base+p.Path, nil)
		if e == nil {
			r.Header.Set("Authorization", "Bearer "+secret.Token)
		}
		return r, e
	}}
	r, e := erpload.Run(context.Background(), client, p.Options, []erpload.Target{target})
	if e != nil {
		return e
	}
	if e = json.NewEncoder(out).Encode(r); e != nil {
		return e
	}
	if r.Total.Requests == 0 || r.Total.Succeeded != r.Total.Requests {
		return fmt.Errorf("some requests failed; see sanitized counts")
	}
	return nil
}
func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
