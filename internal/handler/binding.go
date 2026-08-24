package handler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// bindMessage 把绑定/校验错误转成可读提示。
//
// 直接回 "参数错误" 排查成本极高 —— 前端看不出是哪个字段不合法，
// 后端日志也没记。这里把首个失败字段和规则带出来。
//
// 只暴露字段名和规则名，不回显用户输入的值，避免把敏感内容打回去。
func bindMessage(err error) string {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) && len(verrs) > 0 {
		e := verrs[0]
		return fmt.Sprintf("参数 %s 不合法（规则：%s）", e.Field(), ruleText(e))
	}

	// 类型不匹配要指名道姓。
	//
	// 只回"请求参数格式错误"的话，前后端字段类型对不上时（比如前端发
	// 字符串 "0"、后端建模成 int）完全看不出是哪个字段的问题 ——
	// isFrame 那个 bug 就是这么藏了很久的。
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return fmt.Sprintf("参数 %s 类型不正确，应为 %s，实际收到 %s",
				typeErr.Field, typeErr.Type, typeErr.Value)
		}
		return fmt.Sprintf("请求参数类型不正确，应为 %s，实际收到 %s", typeErr.Type, typeErr.Value)
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Sprintf("请求体不是合法的 JSON（第 %d 字节处）", syntaxErr.Offset)
	}
	return "请求参数格式错误"
}

func ruleText(e validator.FieldError) string {
	switch e.Tag() {
	case "required", "notblank":
		return "必填"
	case "xss":
		return "不能包含脚本字符"
	case "dicttype":
		return "须以小写字母开头，只含小写字母、数字、下划线"
	case "min":
		return "不小于 " + e.Param()
	case "max":
		return "不大于 " + e.Param()
	case "oneof":
		return "取值须为 " + e.Param()
	case "email":
		return "邮箱格式"
	case "len":
		return "长度须为 " + e.Param()
	default:
		if e.Param() != "" {
			return e.Tag() + "=" + e.Param()
		}
		return e.Tag()
	}
}
