package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"time"
)

type privateChild struct {
	cmd            *exec.Cmd
	info           childInfo
	root           string
	stdout, stderr *os.File
	stopped        bool
}

func startChild() *privateChild {
	gen := strconv.FormatInt(time.Now().UnixNano(), 10)
	root := relRoot + "/children/" + gen
	must(os.MkdirAll(root, 0700))
	exe, e := os.Executable()
	must(e)
	expected, e := filepath.Abs(relRoot + "/runner.exe")
	must(e)
	if exe != expected {
		panic(fail("only current private runner can spawn a test server"))
	}
	cmd := exec.Command(exe, "-mode", "server", "-execute", "-root", relRoot, "-config", configPath, "-accounts", accountsPath, "-generation", gen)
	hideProcess(cmd)
	out, e := os.Create(root + "/stdout.log")
	must(e)
	errout, e := os.Create(root + "/stderr.log")
	must(e)
	cmd.Stdout = out
	cmd.Stderr = errout
	must(cmd.Start())
	c := &privateChild{cmd: cmd, root: root, stdout: out, stderr: errout}
	for i := 0; i < 120; i++ {
		if load(root+"/ready.json", &c.info) == nil {
			if c.info.PID != cmd.Process.Pid || c.info.Generation != gen {
				panic(fail("child identity mismatch"))
			}
			r, e := http.Get(c.info.Base + "/health")
			if e == nil {
				r.Body.Close()
				if r.StatusCode == 200 {
					must(save(relRoot+"/current-child.json", c.info))
					return c
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.kill()
	panic(fail("private child did not become ready"))
}
func (c *privateChild) kill() {
	if c.stopped {
		return
	}
	if c.cmd.Process.Pid != c.info.PID && c.info.PID != 0 {
		panic(fail("child ownership changed"))
	}
	_ = c.cmd.Process.Kill()
	_ = c.cmd.Wait()
	c.stdout.Close()
	c.stderr.Close()
	c.stopped = true
}
func (c *privateChild) stop() {
	if c.stopped {
		return
	}
	_ = save(c.root+"/stop.json", map[string]bool{"stop": true})
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
		c.stopped = true
		c.stdout.Close()
		c.stderr.Close()
	case <-time.After(25 * time.Second):
		c.kill()
	}
}
func childAPI(c *privateChild) api {
	cfg, _ := sourceConfig()
	a := sourceAPI(cfg)
	a.base = c.info.Base
	return a
}

func readCycles() []cycleReceipt {
	journalMu.Lock()
	defer journalMu.Unlock()
	f, e := os.Open(relRoot + "/cycles.jsonl")
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	must(e)
	defer f.Close()
	scan := bufio.NewScanner(f)
	out := []cycleReceipt{}
	for scan.Scan() {
		var r cycleReceipt
		must(json.Unmarshal(scan.Bytes(), &r))
		out = append(out, r)
	}
	must(scan.Err())
	return out
}

type stockFact struct{ OnHand, Reserved, Ledgers, Orders, Packages, Events int64 }

func faultStock(f *relFixture, order int64) stockFact {
	_, d := sourceConfig()
	db := openDB(d)
	defer db.Close()
	ctx, c := context.WithTimeout(context.Background(), 15*time.Second)
	defer c()
	var v stockFact
	must(db.QueryRowContext(ctx, "SELECT on_hand,reserved FROM erp_stock WHERE tenant_id=? AND warehouse_id=? AND sku_id=?", f.ID, f.Warehouse, f.SKU).Scan(&v.OnHand, &v.Reserved))
	for _, q := range []struct {
		sql string
		p   *int64
	}{
		{"SELECT COUNT(*) FROM erp_stock_ledger WHERE tenant_id=?", &v.Ledgers},
		{"SELECT COUNT(*) FROM erp_order WHERE tenant_id=?", &v.Orders},
		{"SELECT COUNT(*) FROM erp_package WHERE tenant_id=?", &v.Packages},
		{"SELECT COUNT(*) FROM erp_order_event WHERE tenant_id=?", &v.Events},
	} {
		must(db.QueryRowContext(ctx, q.sql, f.ID).Scan(q.p))
	}
	return v
}

type payFact struct {
	Paid                           string
	Payments, Allocations, Actions int64
}

func faultMoney(f *relFixture, payable int64) payFact {
	_, d := sourceConfig()
	db := openDB(d)
	defer db.Close()
	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	var v payFact
	must(db.QueryRowContext(ctx, "SELECT paid_amount FROM erp_fin_payable WHERE tenant_id=? AND payable_id=?", f.ID, payable).Scan(&v.Paid))
	for _, q := range []struct {
		sql string
		p   *int64
	}{{"SELECT COUNT(*) FROM erp_fin_payment WHERE tenant_id=?", &v.Payments}, {"SELECT COUNT(*) FROM erp_fin_allocation WHERE tenant_id=?", &v.Allocations}, {"SELECT COUNT(*) FROM erp_fin_action WHERE tenant_id=? AND action='PAYMENT_CREATE'", &v.Actions}} {
		must(db.QueryRowContext(ctx, q.sql, f.ID).Scan(q.p))
	}
	return v
}
func provePendingWrites(hit faultHit) int64 {
	_, d := sourceConfig()
	db := openDB(d)
	defer db.Close()
	ctx, c := context.WithTimeout(context.Background(), 4*time.Second)
	defer c()
	for i := 0; i < 30; i++ {
		var modified int64
		e := db.QueryRowContext(ctx, "SELECT trx_rows_modified FROM information_schema.innodb_trx WHERE trx_mysql_thread_id=?", hit.ConnectionID).Scan(&modified)
		if e == nil && modified > 0 {
			return modified
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic(fail("no independent proof of uncommitted modified rows"))
}
func awaitRollback(hit faultHit) {
	_, d := sourceConfig()
	db := openDB(d)
	defer db.Close()
	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	for i := 0; i < 40; i++ {
		var n int
		must(db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.innodb_trx WHERE trx_mysql_thread_id=?", hit.ConnectionID).Scan(&n))
		must(appendEvidence(relRoot+"/rollback-observation.jsonl", map[string]any{"atUtc": time.Now().UTC(), "connectionId": hit.ConnectionID, "poll": i, "transactions": n}))
		if n == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	panic(fail("transaction did not roll back"))
}

func requestWithContext(a api, ctx context.Context, method, path, token string, body any) (map[string]any, error) {
	b, e := json.Marshal(body)
	if e != nil {
		return nil, e
	}
	r, e := http.NewRequestWithContext(ctx, method, a.base+path, bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	res, e := a.client.Do(r)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	var out map[string]any
	e = json.NewDecoder(res.Body).Decode(&out)
	return out, e
}
func runFaults(child **privateChild, s *relState) {
	a := childAPI(*child)
	defer a.cache.Close()
	f := s.Fixtures[1]
	// A real received purchase supplies one payable for the three money cases.
	po := field(data(a.ok("POST", "/erp/purchase", f.Owner, map[string]any{"supplierId": f.Supplier, "warehouseId": f.Warehouse, "currency": "CNY", "requestKey": "rel-fault-po", "lines": []map[string]any{{"skuId": f.SKU, "quantity": 15, "unitPrice": "2.0000"}}})), "purchaseId")
	a.ok("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", po), f.Owner, map[string]any{"version": 1, "requestKey": "rel-fault-confirm"})
	view := data(a.ok("GET", fmt.Sprintf("/erp/purchase/%d", po), f.Owner, nil))
	line := field(view["lines"].([]any)[0].(map[string]any), "lineId")
	receipt := field(data(a.ok("POST", fmt.Sprintf("/erp/purchase/%d/receipts", po), f.Owner, map[string]any{"version": view["version"], "requestKey": "rel-fault-receive", "lines": []map[string]any{{"purchaseLineId": line, "quantity": 15}}})), "receiptId")
	payable := field(data(a.ok("POST", "/erp/finance/payables", f.Owner, map[string]any{"receiptId": receipt, "requestKey": "rel-fault-payable"})), "id")
	results := []map[string]any{}
	for _, kind := range []string{"review", "payment"} {
		for index, mode := range []string{"deadline-before-commit", "kill-before-commit", "restart-after-commit"} {
			label := kind + "-" + mode
			fmt.Println("FAULT: " + label)
			key := "rel-fault-" + label
			var order int64
			method, path, table := "POST", "/erp/finance/payments", "erp_fin_action"
			var body map[string]any
			if kind == "review" {
				order = field(data(a.ok("POST", "/erp/order", f.Owner, orderInput(f, "rel-fault-order-"+strconv.Itoa(index), "FAULT-"+strconv.Itoa(index), 1))), "orderId")
				method, path, table = "PUT", fmt.Sprintf("/erp/order/%d/review", order), "erp_order_event"
				body = map[string]any{"version": 1, "requestKey": key, "warehouseId": f.Warehouse}
			} else {
				v := data(a.ok("GET", fmt.Sprintf("/erp/finance/payables/%d", payable), f.Owner, nil))
				body = map[string]any{"requestKey": key, "accountId": f.Account, "supplierId": f.Supplier, "currency": "CNY", "amount": "5.0000", "allocations": []map[string]any{{"payableId": payable, "version": v["version"], "amount": "5.0000"}}}
			}
			beforeStock, beforeMoney := faultStock(f, order), faultMoney(f, payable)
			var hit faultHit
			var rowsModified int64
			if mode != "restart-after-commit" {
				must(save(relRoot+"/fault-control.json", faultControl{TenantID: f.ID, Key: key, Table: table, Mode: mode}))
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				done := make(chan error, 1)
				go func() {
					v, e := requestWithContext(a, ctx, method, path, f.Owner, body)
					if e == nil && v["code"] == float64(200) {
						e = fail("fault request unexpectedly committed")
					}
					done <- e
				}()
				for i := 0; i < 40; i++ {
					if load((*child).root+"/fault-hit.json", &hit) == nil && hit.Key == key {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
				if hit.Key != key {
					cancel()
					panic(fail("transaction gate not hit"))
				}
				rowsModified = provePendingWrites(hit)
				if faultStock(f, order) != beforeStock || faultMoney(f, payable) != beforeMoney {
					cancel()
					panic(fail("uncommitted facts leaked"))
				}
				must(os.Remove(relRoot + "/fault-control.json"))
				if mode == "kill-before-commit" {
					(*child).kill()
				}
				e := <-done
				cancel()
				if e == nil {
					panic(fail("expected transport failure was not observed"))
				}
				if mode == "deadline-before-commit" && !errors.Is(e, context.DeadlineExceeded) {
					panic(fail("request did not reach actual deadline"))
				}
				awaitRollback(hit)
				if faultStock(f, order) != beforeStock || faultMoney(f, payable) != beforeMoney {
					panic(fail("rollback left partial facts"))
				}
			} else {
				// Consume a real successful response privately, withhold it from the
				// simulated caller, independently prove commit, then kill the child.
				a.ok(method, path, f.Owner, body)
				if kind == "review" {
					if faultStock(f, order).Reserved != beforeStock.Reserved+1 {
						panic(fail("committed review missing"))
					}
				} else {
					if faultMoney(f, payable).Payments != beforeMoney.Payments+1 {
						panic(fail("committed payment missing"))
					}
				}
				(*child).kill()
			}
			if (*child).stopped {
				*child = startChild()
				a.base = (*child).info.Base
			}
			out := data(a.ok(method, path, f.Owner, body))
			wantExisting := mode == "restart-after-commit"
			if out["existing"] != wantExisting {
				panic(fail("wrong restart/retry disposition"))
			}
			if replay := data(a.ok(method, path, f.Owner, body)); replay["existing"] != true {
				panic(fail("second retry did not replay"))
			}
			afterStock, afterMoney := faultStock(f, order), faultMoney(f, payable)
			if kind == "review" {
				if afterStock.Reserved != beforeStock.Reserved+1 || afterStock.OnHand != beforeStock.OnHand || afterStock.Packages != beforeStock.Packages+1 || afterStock.Events != beforeStock.Events+1 || afterMoney != beforeMoney {
					panic(fail("review retry cardinality drift"))
				}
				a.ok("PUT", fmt.Sprintf("/erp/order/%d/cancel", order), f.Owner, map[string]any{"version": out["version"], "requestKey": "cancel-" + key, "reason": "Synthetic fault cleanup"})
			} else {
				if afterMoney.Payments != beforeMoney.Payments+1 || afterMoney.Allocations != beforeMoney.Allocations+1 || afterMoney.Actions != beforeMoney.Actions+1 || afterStock != beforeStock {
					panic(fail("payment retry cardinality drift"))
				}
			}
			results = append(results, map[string]any{"case": label, "passed": true, "rowsModifiedBeforeInterruption": rowsModified, "connectionId": hit.ConnectionID, "rollbackVerified": mode != "restart-after-commit", "committedVerifiedBeforeRestart": mode == "restart-after-commit", "originalBodyAndVersionReused": true, "existingOnFirstRetry": wantExisting})
			must(save(relRoot+"/fault-results.json", results))
		}
	}
	v := data(a.ok("GET", fmt.Sprintf("/erp/finance/payables/%d", payable), f.Owner, nil))
	a.ok("POST", "/erp/finance/payments", f.Owner, map[string]any{"requestKey": "rel-fault-final-pay", "accountId": f.Account, "supplierId": f.Supplier, "currency": "CNY", "amount": "15.0000", "allocations": []map[string]any{{"payableId": payable, "version": v["version"], "amount": "15.0000"}}})
	stock, money := faultStock(f, 0), faultMoney(f, payable)
	if stock.OnHand != 115 || stock.Reserved != 0 || money.Paid != "30.0000" || money.Payments != 4 || money.Allocations != 4 || money.Actions != 4 {
		panic(fail("final fault facts disagree"))
	}
	must(save(relRoot+"/fault-final-facts.json", map[string]any{"stock": stock, "money": money, "allSixPassed": true}))
}
