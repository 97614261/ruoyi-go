// Package types 提供跨层复用的基础类型。
package types

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Layout 前端约定的时间格式，与 Java 版 spring.jackson.date-format 一致。
const Layout = "2006-01-02 15:04:05"

// Location 固定东八区，避免依赖运行环境的 tzdata（Windows 上常缺失）。
var Location = time.FixedZone("Asia/Shanghai", 8*60*60)

// Time 统一 JSON 序列化格式的时间类型。
//
// 直接用 time.Time 会序列化成 RFC3339（2026-08-20T10:00:00+08:00），
// RuoYi 前端的表格和日期组件都解析不了，必须用这个类型。
type Time time.Time

// MarshalJSON 零值输出 null，其余按 Layout 格式化。
func (t Time) MarshalJSON() ([]byte, error) {
	std := time.Time(t)
	if std.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + std.In(Location).Format(Layout) + `"`), nil
}

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*t = Time{}
		return nil
	}
	std, err := time.ParseInLocation(Layout, s, Location)
	if err != nil {
		return fmt.Errorf("时间格式必须为 %q: %w", Layout, err)
	}
	*t = Time(std)
	return nil
}

// Value 实现 driver.Valuer，零值写入 NULL。
func (t Time) Value() (driver.Value, error) {
	std := time.Time(t)
	if std.IsZero() {
		return nil, nil
	}
	return std, nil
}

// Scan 实现 sql.Scanner。
//
// 依赖 DSN 中的 parseTime=True，否则驱动返回 []byte 而不是 time.Time。
func (t *Time) Scan(v any) error {
	switch value := v.(type) {
	case time.Time:
		*t = Time(value)
		return nil
	case nil:
		*t = Time{}
		return nil
	default:
		return fmt.Errorf("无法将 %T 转换为 types.Time（检查 DSN 是否带 parseTime=True）", v)
	}
}

// Now 返回东八区当前时间。
func Now() Time { return Time(time.Now().In(Location)) }

// FromUnixMilli 由 Unix 毫秒构造，0 返回零值而不是 1970 年。
//
// LoginUser 里的 loginTime / expireTime 存的是毫秒时间戳，需要这个转换。
func FromUnixMilli(ms int64) Time {
	if ms == 0 {
		return Time{}
	}
	return Time(time.UnixMilli(ms).In(Location))
}

// Std 转回标准库类型，便于做时间计算。
func (t Time) Std() time.Time { return time.Time(t) }

// IsZero 是否为零值。
func (t Time) IsZero() bool { return time.Time(t).IsZero() }

func (t Time) String() string {
	if t.IsZero() {
		return ""
	}
	return time.Time(t).In(Location).Format(Layout)
}
