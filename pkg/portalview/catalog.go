package portalview

// BuiltinTheme is a trusted code catalog, never supplied by a template upload.
type BuiltinTheme struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

func BuiltinThemes() []BuiltinTheme {
	return []BuiltinTheme{{"business", "企业官网", "v3", "综合介绍、产品与服务，留白清晰"}, {"editorial", "资讯门户", "v1", "栏目阅读与资讯展示"}, {"industrial", "制造与工程", "v1", "工程目录、产品索引与技术内容，紧凑直角布局"}}
}
func FindBuiltinTheme(id string) (BuiltinTheme, bool) {
	for _, t := range BuiltinThemes() {
		if t.ID == id {
			return t, true
		}
	}
	return BuiltinTheme{}, false
}
