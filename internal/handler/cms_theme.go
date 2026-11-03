package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/portalview"
	"ruoyi-go/pkg/response"
)

func CMSThemes(c *gin.Context) {
	pg := page.Parse(c, nil)
	rows := portalview.BuiltinThemes()
	start := pg.Offset()
	if start > len(rows) {
		start = len(rows)
	}
	end := start + pg.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	c.Header("Cache-Control", "no-store")
	response.Page(c, rows[start:end], int64(len(rows)))
}
func (p *PortalUI) CompareThemes(c *gin.Context) {
	var in struct {
		Version   int64  `json:"version" binding:"required,gt=0"`
		Theme     string `json:"theme" binding:"required,max=32"`
		ChannelId int64  `json:"channelId" binding:"min=0"`
		Layout    string `json:"layout" binding:"omitempty,oneof=grid list"`
	}
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "对比参数不正确")
		return
	}
	if in.ChannelId > 0 && !middleware.CurrentUser(c).HasPermission("cms:channel:query") {
		response.FailCode(c, 403, "栏目预览需要栏目查询权限")
		return
	}
	state, e := service.CMSGetSite(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	if state.Version != in.Version {
		response.Fail(c, "外观草稿已变化，请重新读取")
		return
	}
	if _, e = service.ValidateSite(state.Config); e != nil {
		fail(c, e)
		return
	}
	before, e := service.CMSResolveSite(c.Request.Context(), state.Config)
	if e != nil {
		fail(c, e)
		return
	}
	after := before
	after.Theme = in.Theme
	after.ThemeVersion = ""
	after.ChannelLayouts = append([]model.ChannelLayout{}, before.ChannelLayouts...)
	if in.Layout != "" && in.ChannelId > 0 {
		found := false
		for i, o := range after.ChannelLayouts {
			if o.ChannelId == in.ChannelId {
				after.ChannelLayouts[i].Layout = in.Layout
				found = true
			}
		}
		if !found {
			after.ChannelLayouts = append(after.ChannelLayouts, model.ChannelLayout{ChannelId: in.ChannelId, Layout: in.Layout})
		}
	}
	if _, e = service.ValidateSite(after); e != nil {
		fail(c, e)
		return
	}
	var d portalview.PageData
	name := "home"
	if in.ChannelId == 0 {
		d, e = p.homeData(c, before)
	} else {
		ch, err := service.CMSChannelDetail(c.Request.Context(), in.ChannelId)
		if err != nil {
			fail(c, err)
			return
		}
		result, err := service.PortalResults(c.Request.Context(), ch.Kind, ch.Slug, "", 0, 12)
		if err != nil {
			fail(c, err)
			return
		}
		d = portalview.PageData{Title: ch.Name, Description: ch.SeoDescription, Cards: cardRows(result.Rows), IsListing: true, ChannelId: ch.ChannelId}
		name = "column"
	}
	if e != nil {
		fail(c, e)
		return
	}
	d.NoIndex = true
	a, b := d, d
	p.decorate(&a, before)
	p.decorate(&b, after)
	first, e := p.view.Render(before.Theme, name, a)
	if e != nil {
		response.Fail(c, "原模板预览失败")
		return
	}
	second, e := p.view.Render(after.Theme, name, b)
	if e != nil {
		response.Fail(c, "候选模板预览失败")
		return
	}
	left, _ := portalview.FindBuiltinTheme(before.Theme)
	right, _ := portalview.FindBuiltinTheme(after.Theme)
	changes := []string{left.Name + " → " + right.Name, "同一份公开内容快照，外观不保存、不发布"}
	if in.ChannelId > 0 {
		changes = append(changes, fmt.Sprintf("栏目#%d：布局 %s → %s（空值表示模板原布局）", in.ChannelId, a.ListingLayout, b.ListingLayout))
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	response.OkData(c, map[string]any{"version": state.Version, "fromTheme": before.Theme, "toTheme": after.Theme, "differences": changes, "beforeHtml": string(first), "afterHtml": string(second), "scope": "只比较已保存的外观草稿；未保存修改不参与"})
}
