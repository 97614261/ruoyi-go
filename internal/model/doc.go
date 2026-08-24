// Package model GORM 实体与请求/响应 DTO。[L1]
//
// 约定见 docs/CONVENTIONS.md 第六节：
//   - 数据库列 snake_case，Go 字段 PascalCase，JSON camelCase，三者必须显式声明
//   - status / del_flag 是字符串不是数字（"0" 正常 / "1" 停用，del_flag "2" 已删除）
//   - 时间字段用 types.Time，不要用 time.Time
//   - 可空列用指针，不要用零值冒充 NULL
package model
