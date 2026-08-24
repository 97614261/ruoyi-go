package model

// IntValue 解引用可空整数，nil 返回 0。
//
// 【为什么需要它】排序字段（orderNum / roleSort / postSort / dictSort）都是
// *int：只有指针才能同时表达"必填"和"0 是合法值"——
// 非指针 int 分不清"没传"和"传了 0"，会把 Java 拒绝的请求放行。
//
// 但入库时要的是具体数值，而且这些列在数据库里是可空的（读回来可能是 nil），
// 所以写 SQL 前统一过这个函数，不要在业务代码里散落 `if p != nil` 判断。
func IntValue(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// IntPtr 取整数的地址，构造测试数据和填默认值时用。
func IntPtr(v int) *int { return &v }
