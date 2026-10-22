package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"sync"
	"time"
)

// A managed single-worker executor with durable admission (100 global / 20
// tenant jobs). No per-upload goroutines or unbounded in-memory queues.
var orderImports struct {
	sync.Mutex
	root   *os.Root
	limit  int64
	cancel context.CancelFunc
	done   chan struct{}
}
var orderImportAdmission sync.Mutex
var privateImportKey = regexp.MustCompile(`^[a-f0-9]{64}\.xlsx$`)

func StartERPOrderImports(ctx context.Context, cfg config.UploadConfig) error {
	orderImports.Lock()
	defer orderImports.Unlock()
	if orderImports.root != nil {
		return errors.New("order import executor already started")
	}
	available, err := repository.InitERPImportRecovery(ctx)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}
	// A sibling, never a child of the publicly served attachment root.
	dir, err := filepath.Abs(cfg.Path + "-order-import-private")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("private import root cannot be a symlink")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	orderImports.root = root
	orderImports.limit = min(int64(10<<20), cfg.MaxSizeMB<<20)
	orderImports.cancel = cancel
	orderImports.done = make(chan struct{})
	go runERPOrderImports(workerCtx, orderImports.done)
	return nil
}
func ShutdownERPOrderImports(ctx context.Context) error {
	orderImports.Lock()
	cancel, done := orderImports.cancel, orderImports.done
	orderImports.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func privateImportRoot() (*os.Root, int64, error) {
	orderImports.Lock()
	defer orderImports.Unlock()
	if orderImports.root == nil {
		return nil, 0, errs.New("订单导入未初始化，请完成一期结构升级并重启")
	}
	return orderImports.root, orderImports.limit, nil
}
func SubmitERPOrderImport(ctx context.Context, shop int64, key string, r io.Reader, operator string) (*model.ERPImportJob, error) {
	if shop < 1 || !orderRequestKeyPattern.MatchString(key) {
		return nil, errs.New("店铺或请求标识不正确")
	}
	root, limit, err := privateImportRoot()
	if err != nil {
		return nil, err
	}
	limit, err = EffectiveUploadLimit(ctx, limit)
	if err != nil {
		return nil, err
	}
	// Bounded admission also limits simultaneous staging files. No filename/PII
	// from the request is used as a path, original filename, audit, or error.
	orderImportAdmission.Lock()
	defer orderImportAdmission.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	n, err := repository.ERPImportQueueSize(ctx)
	if err != nil {
		return nil, err
	}
	if n >= 100 {
		return nil, errs.New("导入队列繁忙，请稍后再试")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, errs.New("文件读取失败")
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, errs.New("导入文件为空或超过有效上传上限")
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	fileKey := orderHash(uuid.NewString()) + ".xlsx"
	f, err := root.OpenFile(fileKey, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errs.New("导入私有存储不可用")
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = root.Remove(fileKey)
		return nil, errs.New("导入文件保存失败")
	}
	j := model.ERPImportJob{ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now()}, ShopID: shop, ActorUserID: erpctx.UserID(ctx), RequestKey: key, RequestHash: orderHash([]any{shop, hash}), Status: "PENDING", Version: 1, FileKey: fileKey, FileSha256: hash, FileBytes: int64(len(data)), FileState: "PRESENT", RetainUntil: types.Time(time.Now().Add(24 * time.Hour))}
	out, err := repository.InsertERPImportJob(ctx, &j)
	if err != nil || out.FileKey != fileKey {
		if e := root.Remove(fileKey); e != nil {
			slog.Warn("回收本次未登记导入文件失败")
		}
	}
	if out != nil {
		out.Remark = ""
	}
	return out, err
}
func RetryERPOrderImport(ctx context.Context, id int64, in model.ERPImportRetry) (*model.ERPImportJob, error) {
	if in.Version < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) {
		return nil, errs.New("重试须提供版本和有效请求标识")
	}
	if _, _, err := privateImportRoot(); err != nil {
		return nil, err
	}
	orderImportAdmission.Lock()
	defer orderImportAdmission.Unlock()
	n, err := repository.ERPImportQueueSize(ctx)
	if err != nil {
		return nil, err
	}
	if n >= 100 {
		return nil, errs.New("导入队列繁忙")
	}
	out, err := repository.RetryERPImport(ctx, id, in, orderHash(in))
	if out != nil {
		out.Remark = ""
	}
	return out, err
}
func ListERPImportJobs(ctx context.Context, q model.ERPImportQuery, pg page.Query) ([]model.ERPImportJob, int64, error) {
	switch q.Status {
	case "", "PENDING", "RUNNING", "SUCCEEDED", "PARTIAL_FAILED", "FAILED", "INTERRUPTED":
	default:
		return nil, 0, errs.New("任务状态不正确")
	}
	return repository.SelectERPImportJobs(ctx, q, pg)
}
func GetERPImportJob(ctx context.Context, id int64) (*model.ERPImportJob, error) {
	j, err := repository.SelectERPImportJob(ctx, id)
	if j != nil {
		j.Remark = ""
	}
	return j, err
}
func GetERPImportResults(ctx context.Context, id int64, pg page.Query) ([]model.ERPImportResult, int64, error) {
	return repository.SelectERPImportResults(ctx, id, pg)
}
func ERPImportFile(ctx context.Context, id int64) ([]byte, error) {
	j, err := repository.SelectERPImportJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.FileState != "PRESENT" || !j.RetainUntil.Std().After(time.Now()) {
		return nil, errs.New("原文件已过期")
	}
	return readPrivateImport(*j)
}
func readPrivateImport(j model.ERPImportJob) ([]byte, error) {
	root, _, err := privateImportRoot()
	if err != nil {
		return nil, err
	}
	if !privateImportKey.MatchString(j.FileKey) || j.FileBytes < 1 || j.FileBytes > 10<<20 {
		return nil, errs.New("导入文件不可用")
	}
	f, err := root.Open(j.FileKey)
	if err != nil {
		return nil, errs.New("导入文件不存在或已过期")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 10<<20+1))
	if err != nil || int64(len(b)) != j.FileBytes {
		return nil, errs.New("导入文件校验失败")
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != j.FileSha256 {
		return nil, errs.New("导入文件校验失败")
	}
	return b, nil
}
func runERPOrderImports(ctx context.Context, done chan struct{}) {
	defer close(done)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	cleanup := time.NewTicker(time.Minute)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanup.C:
			cleanupERPImportFiles(ctx)
		case <-timer.C:
			func() {
				queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				j, err := repository.ClaimERPImportJob(queryCtx)
				if err != nil {
					slog.Warn("读取订单导入队列失败")
					return
				}
				if j != nil {
					processERPOrderImport(ctx, *j)
				}
			}()
		}
	}
}
func processERPOrderImport(parent context.Context, j model.ERPImportJob) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	ctx = erpctx.WithUser(tenantctx.With(ctx, j.TenantID, false), j.ActorUserID)
	data, err := readPrivateImport(j)
	var groups []orderImportGroup
	var total int64
	if err == nil {
		groups, total, err = parseOrderImport(ctx, data, j.ShopID)
	}
	interrupted := false
	if e := repository.SetERPImportTotal(ctx, j, total); e != nil {
		interrupted = true
		groups = nil
	}
	if err != nil && !interrupted {
		// Never propagate a parser/SQL value to a result, log or browser.
		group := orderImportGroup{Input: model.ERPOrderWrite{ShopID: j.ShopID}, Rows: []int64{1}, Hash: orderHash([]any{j.FileSha256, "invalid-file"}), Invalid: "文件无法按固定模板解析；请检查格式、公式、文本编号及行数限制"}
		groups = []orderImportGroup{group}
	}
	for _, g := range groups {
		if ctx.Err() != nil {
			interrupted = true
			break
		}
		orderCtx, stop := context.WithTimeout(ctx, 15*time.Second)
		_, err = repository.CommitERPImportedOrder(orderCtx, j, g.Input, g.Hash, g.Invalid, g.Rows)
		stop()
		if err != nil {
			interrupted = true
			break
		}
	}
	finishCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	finishCtx = erpctx.WithUser(tenantctx.With(finishCtx, j.TenantID, false), j.ActorUserID)
	if err = repository.FinishERPImportJob(finishCtx, j, total, interrupted); err != nil {
		slog.Warn("保存订单导入终态失败", "jobId", j.JobID)
	}
}
func cleanupERPImportFiles(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if err := repository.InterruptStaleERPImports(ctx); err != nil {
		slog.Warn("恢复超时导入任务失败")
		return
	}
	jobs, err := repository.ExpiredERPImportJobs(ctx)
	if err != nil {
		slog.Warn("读取到期导入文件失败")
		return
	}
	root, _, err := privateImportRoot()
	if err != nil {
		return
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		if !privateImportKey.MatchString(j.FileKey) {
			slog.Warn("拒绝清理非法导入存储标识", "jobId", j.JobID)
			continue
		}
		err = root.Remove(j.FileKey)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("回收导入文件失败", "jobId", j.JobID)
			continue
		}
		if err = repository.ForgetERPImportFile(ctx, j); err != nil {
			slog.Warn("释放导入文件配额失败", "jobId", j.JobID)
		}
	}
	cleanupERPImportOrphans(ctx, root)
}
func cleanupERPImportOrphans(ctx context.Context, root *os.Root) {
	dir, err := root.Open(".")
	if err != nil {
		return
	}
	defer dir.Close()
	for ctx.Err() == nil {
		entries, e := dir.ReadDir(200)
		keys := []string{}
		for _, entry := range entries {
			if entry.Type().IsRegular() && privateImportKey.MatchString(entry.Name()) {
				info, err := entry.Info()
				if err == nil && time.Since(info.ModTime()) > 24*time.Hour {
					keys = append(keys, entry.Name())
				}
			}
		}
		if len(keys) > 0 {
			found, err := repository.ExistingERPImportFileKeys(ctx, keys)
			if err != nil {
				return
			}
			keep := map[string]bool{}
			for _, k := range found {
				keep[k] = true
			}
			for _, k := range keys {
				if !keep[k] {
					if err = root.Remove(k); err != nil && !errors.Is(err, os.ErrNotExist) {
						slog.Warn("回收孤立导入文件失败")
					}
				}
			}
		}
		if e != nil {
			return
		}
	}
}
