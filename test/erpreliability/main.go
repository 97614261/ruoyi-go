package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"ruoyi-go/internal/repository"
)

var relRoot, configPath, accountsPath string
var baselineDuration, cooldownDuration time.Duration

func relQuantile(rows []time.Duration, q float64) float64 {
	if len(rows) == 0 {
		return 0
	}
	copyRows := append([]time.Duration(nil), rows...)
	sort.Slice(copyRows, func(i, j int) bool { return copyRows[i] < copyRows[j] })
	rank := int(math.Ceil(float64(len(rows)) * q))
	if rank < 1 {
		rank = 1
	}
	if rank > len(rows) {
		rank = len(rows)
	}
	return float64(copyRows[rank-1]) / float64(time.Millisecond)
}
func profileTimes(profile string) (time.Duration, time.Duration, time.Duration, error) {
	switch profile {
	case "quick":
		return 15 * time.Second, 2 * time.Minute, 30 * time.Second, nil
	case "standard":
		return 2 * time.Minute, 30 * time.Minute, 5 * time.Minute, nil
	default:
		return 0, 0, 0, fail("profile must be quick or standard")
	}
}
func validateRoot(root string) error {
	absolute, e := filepath.Abs(root)
	if e != nil {
		return fail("invalid evidence path")
	}
	logs, e := filepath.Abs("logs")
	if e != nil {
		return e
	}
	if filepath.Dir(absolute) != logs || !strings.HasPrefix(filepath.Base(absolute), "erp-verify-") || strings.ContainsAny(filepath.Base(absolute), " \t\r\n") {
		return fail("evidence must be a fresh direct logs/erp-verify-* directory")
	}
	info, e := os.Lstat(absolute)
	if e != nil {
		return fail("wrapper-owned evidence directory required")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fail("evidence symlink refused")
	}
	for p := filepath.Dir(absolute); ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return fail("symlink parent refused")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func runPrefix() string { return strings.TrimPrefix(filepath.Base(relRoot), "erp-verify-") }
func ownedFixture(watermark int64, f *relFixture) bool {
	if f == nil || f.ID <= watermark || f.OwnerID < 1 {
		return false
	}
	for i := 0; i < 3; i++ {
		if f.Code == fmt.Sprintf("%s-%d", runPrefix(), i) {
			return true
		}
	}
	return false
}
func validateFixture(watermark int64, f *relFixture) {
	if !ownedFixture(watermark, f) {
		panic(fail("old or foreign fixture refused"))
	}
	t, e := repository.ResolveTenant(context.Background(), f.Code)
	must(e)
	if t == nil || t.TenantID != f.ID || t.OwnerUserID == nil || *t.OwnerUserID != f.OwnerID || t.Status != "0" {
		panic(fail("inactive/foreign fixture refused; never reopen"))
	}
}
func main() { os.Exit(entry()) }
func entry() (exitCode int) {
	mode := flag.String("mode", "plan", "plan/verify/server")
	execute := flag.Bool("execute", false, "explicitly permit local fixtures and owned child processes")
	root := flag.String("root", "", "wrapper-owned fresh evidence root")
	cfg := flag.String("config", "configs/multitenant.local.yml", "private local config")
	accounts := flag.String("accounts", "logs/multitenant-accounts.local.json", "private platform record")
	profile := flag.String("profile", "quick", "quick/standard")
	generation := flag.String("generation", "", "parent-owned child generation")
	flag.Parse()
	before, duration, after, e := profileTimes(*profile)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 2
	}
	if *mode == "plan" {
		fmt.Printf("PLAN ONLY; no files/config/DB/Redis read or written. profile=%s baseline=%s load=%s cooldown=%s; six transaction faults, SQL reconciliation, fresh tenants disabled. Linux runtime is not certified. Use scripts/verify-erp-reliability.ps1 -Execute.\n", *profile, before, duration, after)
		return 0
	}
	if !*execute || (*mode != "verify" && *mode != "server") {
		fmt.Fprintln(os.Stderr, "explicit -execute with verify/server required")
		return 2
	}
	defer func() {
		if recover() != nil {
			exitCode = 1
			fmt.Fprintln(os.Stderr, "Verification failed; retain private evidence. No credentials/raw DB errors printed.")
		}
	}()
	relRoot, configPath, accountsPath = *root, *cfg, *accounts
	baselineDuration, cooldownDuration = before, after
	must(validateRoot(relRoot))
	if *mode == "server" {
		serveReliability(*generation)
		return 0
	}
	if runtime.GOOS != "windows" {
		panic(fail("this monitor requires Windows; Linux runtime excluded"))
	}
	// O_EXCL prevents accidentally reusing an earlier run. No resume/reopen mode.
	lock, e := os.OpenFile(relRoot+"/started.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(e)
	must(lock.Close())
	cfgValue, dsn := sourceConfig()
	initRepo(cfgValue)
	defer repository.Close()
	db := openDB(dsn)
	defer db.Close()
	audit := captureOld(db)
	watermark := audit.Watermark
	defer verifyOld(db, audit)
	must(save(relRoot+"/old-data-before.json", audit))
	must(save(relRoot+"/pre-round-audit.json", map[string]any{"watermarkTenantId": watermark, "atUtc": time.Now().UTC(), "profile": *profile}))
	c := startChild()
	defer func() { c.stop() }()
	// Always attempt bounded cleanup of this run's exact new identities, including
	// partial setup/failure. A failed cleanup prevents PASS and preserves evidence.
	defer func() {
		if _, e := os.Stat(relRoot + "/fixture-cleanup.json"); e == nil {
			return
		}
		var s relState
		if e := load(relRoot+"/fixtures.json", &s); e != nil {
			return
		}
		if c.stopped {
			c = startChild()
		}
		a := childAPI(c)
		defer a.cache.Close()
		closeFixtures(a, s)
	}()
	a := childAPI(c)
	defer a.cache.Close()
	setPhase("smoke")
	s := setupFixtures(a)
	for _, f := range []*relFixture{s.Fixtures[0], s.Fixtures[2]} {
		realCycle(a, f, 0)
	}
	verifyFlowFacts(s, "smoke")
	cycles := readCycles()
	var home, foreign int64
	for _, r := range cycles {
		if r.TenantID == s.Fixtures[0].ID {
			home = r.Order
		} else {
			foreign = r.Order
		}
	}
	a.ok("GET", fmt.Sprintf("/erp/order/%d", home), s.Fixtures[0].Owner, nil)
	a.denied("GET", fmt.Sprintf("/erp/order/%d", foreign), s.Fixtures[0].Owner, nil)
	a.denied("GET", fmt.Sprintf("/erp/order/%d", home), s.Fixtures[2].Owner, nil)
	runFaults(&c, &s)
	a.base = c.info.Base
	verifyFlowFacts(s, "after-faults")
	s.SmokePassed = true
	must(save(relRoot+"/fixtures.json", s))
	stopMonitor := startMonitor(c)
	defer stopMonitor()
	sustainedRun(c, s, duration)
	stopMonitor()
	verifyOld(db, audit)
	must(save(relRoot+"/run-complete.json", map[string]any{"atUtc": time.Now().UTC(), "passed": true, "profile": *profile, "scheduledSeconds": duration.Seconds(), "cyclesIncludingWarmups": len(readCycles()), "summary": timingSummary(), "fixturesDisabled": true, "linuxRuntime": "NOT_RUN", "dbDDLChanged": false}))
	fmt.Println("PASS: six faults, full business cycles, reconciliation, resource samples and fresh-fixture cleanup; local Windows only")
	return 0
}
