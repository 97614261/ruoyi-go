// Package validate 注册自定义校验规则，补齐 Go 与 Java Bean Validation 的差异。
package validate

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// Register 注册自定义规则，必须在构建路由之前调用一次。
func Register() error {
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return errors.New("gin 校验引擎类型异常，无法注册自定义规则")
	}

	rules := map[string]validator.Func{
		"notblank": notBlank,
		"xss":      noHTML,
		"dicttype": dictTypeFormat,
		// 覆盖内置 email，理由见 emailOrEmpty
		"email": emailOrEmpty,
	}
	for tag, fn := range rules {
		if err := engine.RegisterValidation(tag, fn); err != nil {
			return err
		}
	}
	return nil
}

// Struct 手动校验一个结构体，用 binding tag。
//
// 供 Excel 导入这类"数据不是从 HTTP 请求体来"的场景使用 ——
// gin 的自动绑定校验走不到，但规则必须和接口保持一致，不能另写一套。
func Struct(v any) error {
	return binding.Validator.ValidateStruct(v)
}

// notBlank 对齐 Java 的 @NotBlank。
//
// Go 内置的 required 对字符串只判断 ""，"   " 能通过 —— 这比 Java 更宽，
// 会让纯空白的名称、标题落库。凡是 Java 标了 @NotBlank 的字段都要用这个。
func notBlank(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() != reflect.String {
		return false
	}
	return strings.TrimSpace(field.String()) != ""
}

// htmlTagPattern 与 Java 版 XssValidator.HTML_PATTERN 保持一致的意图：识别 HTML 标签。
//
// Java 那边的实现绕了一圈（把所有匹配拼起来再整体 matches），
// 净效果就是"是否含 HTML 标签"，这里直接表达该意图。
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// noHTML 对齐 Java 的 @Xss：字段不得包含 HTML 标签。
//
// 与 Java 一致，空白串直接通过（@Xss 不负责非空校验，
// 需要非空请另外叠加 notblank）。
func noHTML(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if strings.TrimSpace(value) == "" {
		return true
	}
	return !htmlTagPattern.MatchString(value)
}

// stdValidator 独立实例，用来复用内置的邮箱格式判断。
//
// 不能用 gin 那个引擎——email 规则已经被我们覆盖，调它会无限递归。
var stdValidator = validator.New()

// emailOrEmpty 覆盖内置的 email 规则，对齐 Jakarta 的 @Email：**空值合法**。
//
// 两个原因必须覆盖而不是靠调用方写 `omitempty,email`：
//
//  1. Go 内置 email 对空串直接判失败，而 Java 的 @Email 认为 null 和空串都合法
//     （邮箱本来就是选填），不处理会导致"不填邮箱就存不了"。
//  2. `omitempty` 对**非 nil 指针不生效**。前端传 "email": "" 时，
//     *string 是非 nil 的、指向空串，validator 的 hasValue 判定为"有值"，
//     omitempty 不会跳过，照样跑 email 校验。这个坑只在指针字段上出现，
//     普通 string 字段用 omitempty 是有效的 —— 极易漏。
//
// 覆盖之后，`binding:"email"` 在两种字段类型上行为一致，写不错。
func emailOrEmpty(fl validator.FieldLevel) bool {
	value := strings.TrimSpace(fl.Field().String())
	if value == "" {
		return true
	}
	return stdValidator.Var(value, "email") == nil
}

// dictTypePattern 对应 SysDictType.dictType 的 @Pattern。
var dictTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// dictTypeFormat 字典类型必须以小写字母开头，只含小写字母、数字、下划线。
func dictTypeFormat(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true // 非空由 notblank 负责
	}
	return dictTypePattern.MatchString(value)
}
