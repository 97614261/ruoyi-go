package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/modelcatalog"
	"ruoyi-go/pkg/redisx"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type doctorReport struct {
	Status      string        `json:"status"`
	VendorCalls bool          `json:"vendorCalls"`
	Checks      []doctorCheck `json:"checks"`
}

func (r *doctorReport) add(name, status, message string) {
	r.Checks = append(r.Checks, doctorCheck{name, status, message})
	if status == "failed" || status == "warning" && r.Status != "failed" {
		r.Status = status
	}
}
func (r *doctorReport) write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	if r.Status == "failed" {
		return errors.New("就绪检查有失败项，请按安全报告处理；未自动修改配置或数据")
	}
	return nil
}

// Only local reads: no cache claims, queue repair, file probes or vendor requests.
func doctorFile(path string, out io.Writer) error {
	r := doctorReport{Status: "passed"}
	cfg, err := config.Load(path)
	if err != nil {
		r.add("config", "failed", "独立配置无法读取或解析；请在本机核对，不要公开配置正文")
		return r.write(out)
	}
	if config.ValidatePOD(cfg) != nil {
		r.add("config", "failed", "独立数据库、缓存、路径、密钥或预算配置不满足 POD 约束")
		return r.write(out)
	}
	r.add("config", "passed", "独立配置静态校验通过；不输出连接串、路径或密钥")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if repository.Init(cfg.MySQL) != nil {
		r.add("database", "failed", "专用数据库不可连接")
		return r.write(out)
	}
	defer repository.Close()
	if repository.VerifyPODSchema(ctx, cfg.POD.InstanceID) != nil {
		r.add("schema", "failed", "实例或 001—015 schema 校验失败；未迁移")
		return r.write(out)
	}
	r.add("schema", "passed", "专用实例与 001—015 schema 校验通过")
	if redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}) != nil {
		r.add("redis", "failed", "独立缓存不可连接")
	} else {
		defer redisx.Close()
		owner, e := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
		if e != nil || owner != cfg.POD.InstanceID || redisx.VerifyPODQueueRedis(ctx) != nil {
			r.add("redis", "failed", "缓存归属或队列版本不满足要求；未认领或清空缓存")
		} else {
			r.add("redis", "passed", "缓存归属和队列版本通过")
			if n, e := redisx.C().Exists(ctx, "pod:worker:heartbeat:"+cfg.POD.InstanceID).Result(); e != nil || n != 1 {
				r.add("worker", "warning", "未发现 Worker 心跳；离线备份时正常，创作服务开放前应启动 Worker")
			} else {
				r.add("worker", "passed", "发现 Worker 心跳；不代表每个任务均已完成")
			}
		}
	}
	for _, dir := range []struct{ name, path string }{{"privateStorage", cfg.POD.StorageRoot}, {"catalogStorage", cfg.POD.StorageRoot + string(os.PathSeparator) + "catalog"}} {
		if info, e := os.Stat(dir.path); e != nil || !info.IsDir() {
			r.add(dir.name, "warning", "素材目录不存在或不可读；请由部署者核对目录归属")
		} else {
			r.add(dir.name, "passed", "素材目录存在；本检查只读，不验证剩余磁盘、写入和硬链接能力")
		}
	}
	if cfg.POD.Execution.Mode == "managed" {
		doctorCatalog(ctx, cfg, &r)
	} else {
		r.add("execution", "warning", "当前不是 managed；正式模型目录与点数执行需另行核对")
	}
	r.add("vendorQuota", "warning", "未访问供应商：Key 有效性、免费额度、账单和真实生图仍须小额验收")
	if cfg.POD.Execution.ImageAIURL != "" {
		r.add("imageAI", "warning", "已配置本机图片 AI；需另验服务健康、权重校验和处理样本，本检查不发起推理")
	}
	return r.write(out)
}

func doctorCatalog(ctx context.Context, cfg *config.Config, r *doctorReport) {
	r.add("inlineImageReceipt", "warning", "Base64 图片回执写入私有素材目录；Worker 在付费调用前检查目录能否写入，不需要调大 MySQL 单包")
	vault, err := modelcatalog.NewVault(cfg.POD.Catalog.KeyringFile, cfg.JWT.Secret, cfg.POD.Catalog.CredentialEnvs)
	if err != nil || !vault.Ready() {
		r.add("vault", "failed", "主密钥不可用；核对独立 keyring 与历史 keyId，不要重建替换旧主密钥")
		return
	}
	r.add("vault", "passed", "独立主密钥可读")
	tx := repository.DB(ctx).Session(&gorm.Session{Logger: logger.Discard})
	var rows []model.PODCatalogModel
	if tx.Model(&model.PODCatalogModel{}).Select("pod_model.*").Joins("JOIN pod_provider p ON p.provider_id=pod_model.provider_id").Where("pod_model.status='enabled' AND pod_model.visible=1 AND p.status='enabled'").Order("pod_model.model_id").Find(&rows).Error != nil {
		r.add("models", "failed", "模型目录不可读取")
		return
	}
	usable := map[string]string{}
	counts := map[string]int{}
	for _, m := range rows {
		if repository.PODCatalogCheck(ctx, m.ModelID, vault) != nil {
			r.add("model:"+strconv.FormatInt(m.ModelID, 10), "failed", "已启用模型的协议、版本或服务端凭据静态检查失败")
			continue
		}
		usable[strconv.FormatInt(m.ModelID, 10)] = m.Kind
		counts[m.Kind]++
	}
	for _, kind := range []string{"text", "image"} {
		status, message := "passed", "静态可用模型数："+strconv.Itoa(counts[kind])+"；不证明供应商可调用或每人均已获授权"
		if counts[kind] == 0 {
			status, message = "warning", "没有已启用且可见的 "+kind+" 模型"
		}
		r.add(kind+"Models", status, message)
	}
	defaults, err := repository.PODCatalogDefaults(ctx)
	if err != nil {
		r.add("defaults", "failed", "默认模型配置不可读")
	} else {
		if !defaults.ManagedEnabled {
			r.add("managedGate", "failed", "平台 managed 开关尚未启用")
		}
		for _, entry := range []struct{ kind, id string }{{"text", defaults.TextModelID}, {"image", defaults.ImageModelID}} {
			status, msg := doctorDefault(entry.kind, entry.id, usable)
			r.add(entry.kind+"Default", status, msg)
		}
	}
	var cost struct{ Charged, Held int64 }
	if tx.Table("pod_provider_call").Select("COALESCE(SUM(charged_micros),0) charged,COALESCE(SUM(reserved_micros),0) held").Take(&cost).Error != nil {
		r.add("budget", "failed", "累计费用账本不可读取")
	} else if cost.Charged+cost.Held >= cfg.POD.Qwen.BudgetFen*10000 {
		r.add("budget", "warning", "平台累计估算预算已用尽或预占；不自动释放未知调用或提额")
	} else {
		r.add("budget", "passed", "平台累计估算预算尚有空间；单次报价/领取仍需服务端核对")
	}
}
func doctorDefault(kind, id string, usable map[string]string) (string, string) {
	if id == "" {
		return "warning", "未设置默认型号，创作前需显式选择模型"
	}
	if usable[id] != kind {
		return "failed", "默认型号不在本类别静态可用目录内"
	}
	return "passed", "默认型号静态检查通过"
}
