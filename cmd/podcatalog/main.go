// podcatalog installs reviewed model presets without credentials or provider calls.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/modelcatalog"
)

type preset struct {
	Provider, Code, Name, Kind string
	OfficialModelID            string `json:"officialModelId"`
	Capabilities               model.PODModelCapabilities
	Cost                       *struct{ Input, Output, Image string }
	Source, Note, CheckedAt    string
}
type providerSpec struct{ Code, Name, Adapter, Region, Endpoint string }

var providers = map[string]providerSpec{
	"deepseek":    {"deepseek", "DeepSeek", "openai-chat-v1", "cn", "https://api.deepseek.com"},
	"zhipu":       {"zhipu-glm", "智谱 GLM", "openai-chat-v1", "cn", "https://open.bigmodel.cn"},
	"siliconflow": {"siliconflow", "硅基流动", "openai-chat-v1", "cn", "https://api.siliconflow.cn"},
	"kimi":        {"kimi", "Kimi / Moonshot", "openai-chat-v1", "cn", "https://api.moonshot.cn"},
	"volcengine":  {"volcengine-ark", "火山引擎 / 火山方舟", "openai-chat-v1", "cn-beijing", "https://ark.cn-beijing.volces.com"},
	"openai":      {"openai", "OpenAI", "openai-chat-v1", "global", "https://api.openai.com"},
}
var money = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$`)

func micros(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	if !money.MatchString(s) {
		return 0, errors.New("invalid decimal CNY price")
	}
	p := strings.SplitN(s, ".", 2)
	fraction := ""
	if len(p) == 2 {
		fraction = p[1]
	}
	n, e := strconv.ParseInt(p[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
	if e != nil || n > 1_000_000_000_000 {
		return 0, errors.New("CNY price exceeds limit")
	}
	return n, nil
}

func versionRequest(p preset) (model.PODCatalogWrite, error) {
	r := model.PODCatalogWrite{OfficialModelID: p.OfficialModelID, TimeoutSeconds: 300, Capabilities: &p.Capabilities, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros"}, PointRule: &model.PODModelRate{Version: 1, Unit: "points"}}
	if p.Cost != nil {
		if (p.Kind == "image" && p.Cost.Image == "") || (p.Kind == "text" && (p.Cost.Input == "" || p.Cost.Output == "")) {
			return r, errors.New("blank is not a free price")
		}
		var err error
		r.CostRule.InputPerMillion, err = micros(p.Cost.Input)
		if err != nil {
			return r, err
		}
		r.CostRule.OutputPerMillion, err = micros(p.Cost.Output)
		if err != nil {
			return r, err
		}
		r.CostRule.PerImage, err = micros(p.Cost.Image)
		if err != nil {
			return r, err
		}
		// Initial retail points: 100 points / CNY. Operators can publish a new version.
		r.PointRule.InputPerMillion = (r.CostRule.InputPerMillion + 9999) / 10000
		r.PointRule.OutputPerMillion = (r.CostRule.OutputPerMillion + 9999) / 10000
		r.PointRule.PerImage = (r.CostRule.PerImage + 9999) / 10000
	}
	return r, modelcatalog.ValidateSpec(p.Kind, r)
}

func loadPresets(path string) ([]preset, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("preset file unavailable")
	}
	var rows []preset
	if json.Unmarshal(b, &rows) != nil || len(rows) == 0 || len(rows) > 100 {
		return nil, errors.New("invalid preset file")
	}
	seen := map[string]bool{}
	for _, p := range rows {
		s, ok := providers[p.Provider]
		if !ok || seen[p.Code] || p.Code == "" {
			return nil, errors.New("unknown provider or repeated preset")
		}
		seen[p.Code] = true
		r, e := versionRequest(p)
		if e != nil {
			return nil, fmt.Errorf("preset %s: %w", p.Code, e)
		}
		provider := model.PODProvider{ProviderID: 1, AdapterKind: s.Adapter, Region: s.Region, Endpoint: s.Endpoint}
		v := model.PODCatalogModelVersion{ModelID: 1, OfficialModelID: p.OfficialModelID, AdapterKind: s.Adapter, AdapterVersion: "1", Endpoint: s.Endpoint, Region: s.Region, TimeoutSeconds: r.TimeoutSeconds}
		v.CapabilitiesJSON, _ = json.Marshal(r.Capabilities)
		v.CostRuleJSON, _ = json.Marshal(r.CostRule)
		v.PointRuleJSON, _ = json.Marshal(r.PointRule)
		v.SpecHash = modelcatalog.SpecHash(1, v, r)
		if e = modelcatalog.ValidateVersion(provider, model.PODCatalogModel{ModelID: 1, Kind: p.Kind}, v); e != nil {
			return nil, fmt.Errorf("preset %s: %w", p.Code, e)
		}
	}
	return rows, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "plan", "plan or apply (local POD development database only)")
	configPath := flag.String("config", "configs/pod.local.yml", "dedicated local POD config")
	data := flag.String("data", "web/src/utils/pod-model-presets.json", "reviewed public preset file")
	providerFilter := flag.String("provider", "", "only prepare presets for this provider profile")
	flag.Parse()
	if *mode != "plan" && *mode != "apply" {
		return errors.New("mode must be plan or apply")
	}
	rows, err := loadPresets(*data)
	if err != nil {
		return err
	}
	if *providerFilter != "" {
		filtered := make([]preset, 0, len(rows))
		for _, row := range rows {
			if row.Provider == *providerFilter {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) == 0 {
			return errors.New("no presets for selected provider")
		}
		rows = filtered
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return errors.New("configuration unavailable")
	}
	if err = config.ValidatePOD(cfg); err != nil {
		return err
	}
	dsn, parseErr := mysql.ParseDSN(cfg.MySQL.DSN)
	if parseErr != nil {
		return errors.New("invalid local database configuration")
	}
	host, _, parseErr := net.SplitHostPort(dsn.Addr)
	if cfg.POD.DatabaseName != "pod_studio_dev" || dsn.DBName != "pod_studio_dev" || parseErr != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return errors.New("only pod_studio_dev is permitted")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		return errors.New("database unavailable")
	}
	defer repository.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = repository.VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
		return err
	}
	if err = service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
		return errors.New("catalog vault unavailable")
	}
	if *mode == "plan" {
		known := 0
		for _, p := range rows {
			if p.Cost != nil {
				known++
			}
		}
		fmt.Printf("%d presets: %d with reviewed CNY prices; %d without prices. Apply creates disabled models only; existing credentials, defaults and grants are preserved. No provider calls.\n", len(rows), known, len(rows)-known)
		return nil
	}
	var before struct {
		Providers []model.PODProvider
		Models    []model.PODCatalogModel
		Versions  []model.PODCatalogModelVersion
	}
	for _, q := range []struct{ Dest any }{{&before.Providers}, {&before.Models}, {&before.Versions}} {
		if err = repository.DB(ctx).Find(q.Dest).Error; err != nil {
			return err
		}
	}
	dir := filepath.Join(".local", "catalog-backups")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	backup := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+uuid.NewString()[:8]+".json")
	b, _ := json.MarshalIndent(before, "", "  ")
	if err = os.WriteFile(backup, b, 0600); err != nil {
		return err
	}
	admin := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1, UserName: "admin"}, Permissions: []string{"*:*:*"}}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	write := func(op string, target int64, r model.PODCatalogWrite) (*model.PODCatalogResult, error) {
		r.RequestID = uuid.NewString()
		r.Reason = "本地模型目录预设维护；不调用模型、不配置凭据"
		return service.PODCatalogWrite(ctx, admin, op, target, r)
	}
	providerIDs := map[string]int64{}
	for _, key := range []string{"deepseek", "zhipu", "siliconflow", "kimi", "openai", "volcengine"} {
		if *providerFilter != "" && key != *providerFilter {
			continue
		}
		s := providers[key]
		var existing []model.PODProvider
		if err = repository.DB(ctx).Where("provider_code=?", s.Code).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) == 0 {
			if _, err = write("provider.create", 0, model.PODCatalogWrite{Code: s.Code, Name: s.Name, AdapterKind: s.Adapter, Region: s.Region, Endpoint: s.Endpoint}); err != nil {
				return err
			}
		}
		var p model.PODProvider
		if err = repository.DB(ctx).Where("provider_code=?", s.Code).Take(&p).Error; err != nil {
			return err
		}
		if p.AdapterKind != s.Adapter || p.Endpoint != s.Endpoint || p.Region != s.Region {
			return fmt.Errorf("provider %s changed; review required", s.Code)
		}
		if p.Name != s.Name {
			if _, err = write("provider.update", p.ProviderID, model.PODCatalogWrite{Revision: id(p.Revision), Name: s.Name}); err != nil {
				return err
			}
		}
		providerIDs[key] = p.ProviderID
	}
	result := []map[string]any{}
	for i, p := range rows {
		var existing []model.PODCatalogModel
		if err = repository.DB(ctx).Where("model_code=?", p.Code).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) == 0 {
			visible := false
			sort := 100 + i
			if _, err = write("model.create", 0, model.PODCatalogWrite{Code: p.Code, Name: p.Name, ProviderID: id(providerIDs[p.Provider]), Kind: p.Kind, Visible: &visible, Sort: &sort}); err != nil {
				return err
			}
		}
		var m model.PODCatalogModel
		if err = repository.DB(ctx).Where("model_code=?", p.Code).Take(&m).Error; err != nil {
			return err
		}
		if m.ProviderID != providerIDs[p.Provider] || m.Kind != p.Kind {
			return fmt.Errorf("model %s changed; review required", p.Code)
		}
		if m.Name != p.Name {
			r, e := write("model.update", m.ModelID, model.PODCatalogWrite{Revision: id(m.Revision), Name: p.Name})
			if e != nil {
				return e
			}
			m.Revision = r.Revision
		}
		if m.CurrentVersionID == nil && p.Cost != nil {
			r, e := versionRequest(p)
			if e != nil {
				return e
			}
			r.Revision = id(m.Revision)
			if _, e = write("model.version", m.ModelID, r); e != nil {
				return fmt.Errorf("version %s: %w", p.Code, e)
			}
		}
		state := "missing-api-key-and-verification"
		if p.Cost == nil {
			state = "missing-cny-price-and-api-key"
		}
		result = append(result, map[string]any{"code": p.Code, "modelId": id(m.ModelID), "provider": p.Provider, "officialModelId": p.OfficialModelID, "state": state, "checkedAt": p.CheckedAt, "source": p.Source, "note": p.Note})
	}
	b, _ = json.MarshalIndent(map[string]any{"vendorCalls": false, "backup": backup, "models": result}, "", "  ")
	resultFile := ".local/catalog-seed-result-20261118.json"
	if *providerFilter != "" {
		resultFile = filepath.Join(".local", "catalog-seed-result-"+*providerFilter+".json")
	}
	if err = os.WriteFile(resultFile, b, 0600); err != nil {
		return err
	}
	fmt.Printf("Applied %d presets; backup: %s. No credentials, defaults, user grants or existing model versions changed. New models remain disabled. No provider calls.\n", len(rows), backup)
	return nil
}
