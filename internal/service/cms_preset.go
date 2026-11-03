package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strings"
)

func cmsPresetCatalog() []model.CMSPreset {
	pages := func(industry string) []model.CMSPresetPage {
		return []model.CMSPresetPage{{Key: "about", Title: "关于我们", Summary: "介绍企业定位、发展经历与团队。", BodyHTML: "<h2>关于我们</h2><p>请在这里填写" + industry + "企业的真实介绍、经历和服务范围。</p>"}, {Key: "services", Title: "服务能力", Summary: "说明服务项目、交付流程与合作方式。", BodyHTML: "<h2>服务能力</h2><p>请填写真实的服务项目、交付标准与支持方式。</p>"}, {Key: "contact", Title: "联系我们", Summary: "填写对外联系方式及工作时间。", BodyHTML: "<h2>联系我们</h2><p>请填写你希望公开的联系电话、邮箱、地址与工作时间。</p>"}}
	}
	blocks := func(title, text string) []model.SiteBlock {
		return []model.SiteBlock{{Kind: "SERVICE", Title: title, Layout: "grid", Enabled: true, Items: []model.SiteWidgetItem{{Title: "了解我们的服务", Text: text}}}, {Kind: "ARTICLE", Title: "最新动态", Count: 6, Enabled: true}, {Kind: "PAGE", Title: "关于与服务", Count: 3, Enabled: true}}
	}
	return []model.CMSPreset{{Id: "manufacturing", Name: "制造与工程", Description: "产品、工程服务与企业介绍，适合制造业和技术服务。", Version: 1, Theme: "industrial", Palette: "ocean", Pages: pages("制造与工程"), Blocks: append([]model.SiteBlock{{Kind: "PRODUCT", Title: "产品展示", Count: 6, Enabled: true}}, blocks("工程与服务", "请填写实际产品优势与技术能力。")...)}, {Id: "business-services", Name: "企业与专业服务", Description: "服务项目、合作方式与企业动态，适合咨询和专业服务。", Version: 1, Theme: "business", Palette: "forest", Pages: pages("专业服务"), Blocks: blocks("我们的服务", "请填写实际服务内容、流程与交付标准。")}, {Id: "news-content", Name: "资讯与内容", Description: "文章和内容阅读优先，适合资讯、知识和内容站点。", Version: 1, Theme: "editorial", Palette: "earth", Pages: pages("内容与资讯"), Blocks: []model.SiteBlock{{Kind: "ARTICLE", Title: "最新文章", Count: 12, Enabled: true}, {Kind: "FAQ", Title: "阅读与联系", Enabled: true, Items: []model.SiteWidgetItem{{Title: "内容如何联系？", Text: "请填写你的真实联系或咨询方式。"}}}}}}
}
func CMSPresetDefinitions(ctx context.Context, pg page.Query) ([]model.CMSPreset, int64, error) {
	if e := repository.CMSPresetAccess(ctx, false); e != nil {
		return nil, 0, e
	}
	all := cmsPresetCatalog()
	start := pg.Offset()
	if start > len(all) {
		start = len(all)
	}
	end := start + pg.PageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], int64(len(all)), nil
}
func CMSPresetDefinition(ctx context.Context, id string) (model.CMSPreset, error) {
	if e := repository.CMSPresetAccess(ctx, false); e != nil {
		return model.CMSPreset{}, e
	}
	for _, p := range cmsPresetCatalog() {
		if p.Id == id {
			return p, nil
		}
	}
	return model.CMSPreset{}, errs.ErrNotFound
}
func cmsBuildPreset(in model.CMSPresetInput, state model.SiteState) (model.CMSPresetPlan, error) {
	out := model.CMSPresetPlan{CMSPresetInput: in, Pages: []model.CMSPresetPage{}, Conflicts: []string{}, Warnings: []string{"只修改已保存的外观草稿；线上保持当前发布版。", "保留站名、联系信息、Logo、轮播、统计设置和原导航；应用后请编辑示例文字。", "新增页面必须单独审核/发布，再审核/发布站点外观。"}}
	if state.Version != in.SiteVersion {
		return out, errs.New("站点草稿已变化，请重新预览")
	}
	in.SlugPrefix = strings.TrimSpace(in.SlugPrefix)
	if len(in.SlugPrefix) > 40 || !cmsSlug.MatchString(in.SlugPrefix) {
		return out, errs.New("页面地址前缀需小写字母开头，只含字母、数字和连字符，最多40位")
	}
	out.CMSPresetInput = in
	var def *model.CMSPreset
	catalog := cmsPresetCatalog()
	for i := range catalog {
		if catalog[i].Id == in.PresetId {
			def = &catalog[i]
			break
		}
	}
	if def == nil {
		return out, errs.ErrNotFound
	}
	out.PresetVersion = def.Version
	selected := map[string]bool{}
	for _, key := range in.PageKeys {
		if selected[key] || (key != "about" && key != "services" && key != "contact") {
			return out, errs.New("页面选择重复或无效")
		}
		selected[key] = true
	}
	// Deep copy before editing slices; a preview can never mutate the source state.
	raw, _ := json.Marshal(state.Config)
	if e := json.Unmarshal(raw, &out.Config); e != nil {
		return out, e
	}
	cfg := &out.Config
	cfg.Theme = def.Theme
	cfg.Palette = def.Palette
	cfg.Density = "normal"
	cfg.ListingLayout = "grid"
	cfg.ChannelLayouts = []model.ChannelLayout{}
	cfg.FontScale = "standard"
	cfg.FrameWidth = "normal"
	cfg.Radius = "soft"
	cfg.Blocks = def.Blocks
	for _, p := range def.Pages {
		if selected[p.Key] {
			p.Slug = in.SlugPrefix + "-" + p.Key
			// Company name is text only even if an older draft contains unusual input.
			p.BodyHTML += "<p>企业名称：" + html.EscapeString(state.Config.SiteName) + "</p>"
			out.Pages = append(out.Pages, p)
			cfg.Navigation = append(cfg.Navigation, model.SiteLink{Label: p.Title, Placement: "header", URL: "/p/" + p.Slug})
		}
	}
	if _, e := ValidateSite(*cfg); e != nil {
		return out, e
	}
	digest, _ := json.Marshal(struct {
		Input   model.CMSPresetInput
		Version int
		Config  model.SiteConfig
		Pages   []model.CMSPresetPage
	}{out.CMSPresetInput, def.Version, *cfg, out.Pages})
	out.PlanSHA256 = fmt.Sprintf("%x", sha256.Sum256(digest))
	return out, nil
}
func CMSPreviewPreset(ctx context.Context, in model.CMSPresetInput) (model.CMSPresetPlan, error) {
	return repository.CMSPreviewPreset(ctx, in, func(s model.SiteState) (model.CMSPresetPlan, error) { return cmsBuildPreset(in, s) })
}
func CMSApplyPreset(ctx context.Context, in model.CMSPresetApply) (model.CMSPresetApplication, error) {
	if !cmsWorkflowText(in.Reason) {
		return model.CMSPresetApplication{}, errs.New("原因含无效字符")
	}
	body, _ := json.Marshal(in)
	hash := fmt.Sprintf("%x", sha256.Sum256(body))
	return repository.CMSApplyPreset(ctx, in, hash, func(s model.SiteState) (model.CMSPresetPlan, error) { return cmsBuildPreset(in.CMSPresetInput, s) })
}
func CMSPresetHistory(ctx context.Context, pg page.Query) ([]model.CMSPresetApplication, int64, error) {
	return repository.CMSPresetHistory(ctx, pg)
}
