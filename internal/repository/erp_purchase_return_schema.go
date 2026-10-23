package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"ruoyi-go/pkg/tenantctx"
	erpsql "ruoyi-go/sql/erp"
)

const ERPPurchaseReturnVersion = "005_purchase_returns"

var purchaseReturnBusinessTables = []string{
	"erp_purchase_return_order", "erp_purchase_return_line",
}

func ERPPurchaseReturnSHA256() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(erpsql.PurchaseReturns, "\r\n", "\n"))))
}

func purchaseReturnManifest() ([]phase1Table, error) {
	return parseERPCreateManifest(erpsql.PurchaseReturns, purchaseReturnBusinessTables)
}

func inspectPurchaseReturns(q *gorm.DB) (*ERPPhase1SchemaStatus, error) {
	manifest, err := purchaseReturnManifest()
	if err != nil {
		return nil, err
	}
	state, err := inspectERPCreateSchema(q, manifest, ERPPurchaseReturnVersion, ERPPurchaseReturnSHA256())
	if err != nil {
		return nil, fmt.Errorf("采购退货结构校验失败: %w", err)
	}
	return state, nil
}

func InspectERPPurchaseReturn(ctx context.Context) (*ERPPhase1SchemaStatus, error) {
	return inspectPurchaseReturns(DB(tenantctx.System(ctx)))
}

// Entirely absent is optional. Partial tables, an orphan version record, drift
// or missing prerequisites fail closed. Startup only READS; it never installs.
func VerifyERPPurchaseReturnSchema(ctx context.Context) error {
	s, err := InspectERPPurchaseReturn(ctx)
	if err != nil {
		return err
	}
	if s.InstalledTables == 0 && !s.Recorded {
		return nil
	}
	if len(s.MissingTables) != 0 || !s.Recorded {
		return fmt.Errorf("采购退货结构未完整登记，请先备份并显式运行erppurchasereturn")
	}
	return requirePurchaseReturnPrerequisites(ctx)
}

func requirePurchaseReturnPrerequisites(ctx context.Context) error {
	if err := VerifyTenantSchema(ctx); err != nil {
		return err
	}
	if err := VerifyERPControls(ctx); err != nil {
		return err
	}
	p, err := InspectERPPhase1(ctx)
	if err != nil {
		return err
	}
	if len(p.MissingTables) != 0 || !p.Recorded {
		return fmt.Errorf("采购退货结构依赖已完整安装并登记的ERP一期003")
	}
	purchasing, err := InspectERPPurchasing(ctx)
	if err != nil {
		return err
	}
	if len(purchasing.MissingTables) != 0 || !purchasing.Recorded {
		return fmt.Errorf("采购退货结构依赖已完整安装并登记的004")
	}
	return nil
}

// Explicit local-only CREATE installer. CLI additionally requires confirmation
// and fresh backup evidence; the operator keeps writers stopped. No grants,
// seeds, ALTER, DROP, repair, or server startup side effect is performed here.
func InstallERPPurchaseReturn(ctx context.Context) (*ERPPhase1SchemaStatus, error) {
	q := DB(tenantctx.System(ctx))
	if err := phase1LocalConnection(q); err != nil {
		return nil, err
	}
	var name string
	if err := q.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
		return nil, err
	}
	if !phase1DatabaseAllowed(name) {
		return nil, fmt.Errorf("采购退货升级拒绝原库或未知数据库")
	}
	if err := requirePurchaseReturnPrerequisites(ctx); err != nil {
		return nil, err
	}
	var result *ERPPhase1SchemaStatus
	err := q.Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{NewDB: true})
		// Share 003's lock to serialize both installers, including older CLI
		// builds, rather than allowing competing schema versions on this DB.
		lockName := name + ":erp-phase1"
		var locked int
		if err := conn.Raw("SELECT GET_LOCK(?,0)", lockName).Scan(&locked).Error; err != nil {
			return err
		}
		if locked != 1 {
			return fmt.Errorf("另一个ERP结构升级正在运行")
		}
		defer func() {
			release, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn.WithContext(tenantctx.System(release)).Session(&gorm.Session{NewDB: true}).Exec("DO RELEASE_LOCK(?)", lockName)
		}()
		state, err := inspectPurchaseReturns(conn)
		if err != nil {
			return err
		}
		// A published record with missing tables is corruption, not an
		// interrupted first installation. Never recreate missing live tables.
		if state.Recorded && len(state.MissingTables) != 0 {
			return fmt.Errorf("采购退货已登记结构缺表，拒绝自动重建，请核对恢复方案")
		}
		missing := map[string]bool{}
		for _, n := range state.MissingTables {
			missing[n] = true
		}
		manifest, err := purchaseReturnManifest()
		if err != nil {
			return err
		}
		for _, table := range manifest {
			if missing[table.Name] {
				if err := conn.Session(&gorm.Session{NewDB: true}).Exec(table.DDL).Error; err != nil {
					return fmt.Errorf("采购退货建表失败：%s；保留部分结构，核对后重跑: %w", table.Name, err)
				}
			}
		}
		state, err = inspectPurchaseReturns(conn)
		if err != nil {
			return err
		}
		if len(state.MissingTables) != 0 {
			return fmt.Errorf("采购退货结构仍不完整")
		}
		if !state.Recorded {
			if err = conn.Exec("INSERT INTO erp_schema_migration(version_key,ddl_sha256,create_by,create_time,remark) VALUES (?,?,?,NOW(),?)", ERPPurchaseReturnVersion, ERPPurchaseReturnSHA256(), "erppurchasereturn", "explicit local CREATE only; no grants or business seeds").Error; err != nil {
				return err
			}
		}
		result, err = inspectPurchaseReturns(conn)
		return err
	})
	return result, err
}

func ERPPurchaseReturnTableNames() []string {
	names := append([]string(nil), purchaseReturnBusinessTables...)
	sort.Strings(names)
	return names
}
