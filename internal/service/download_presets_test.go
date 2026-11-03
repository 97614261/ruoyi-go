package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func downloadZipFixture(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, name := range names {
		f, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte("<document>example</document>"))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestDownloadFileFormatBoundaries(t *testing.T) {
	valid := []struct {
		name string
		b    []byte
	}{{"资料.PDF", []byte("%PDF-1.7\n%%EOF")}, {"指南.txt", []byte("中文说明\nUTF-8 text")}, {"资料.docx", downloadZipFixture(t, "[Content_Types].xml", "word/document.xml")}, {"表格.xlsx", downloadZipFixture(t, "[Content_Types].xml", "xl/workbook.xml")}, {"资料.zip", downloadZipFixture(t, "docs/readme.txt")}}
	for _, v := range valid {
		if _, _, e := validateDownload(v.name, v.b); e != nil {
			t.Fatal(v.name, e)
		}
	}
	invalid := []struct {
		name string
		b    []byte
	}{{"../x.pdf", valid[0].b}, {"x\r.pdf", valid[0].b}, {"x.exe", valid[0].b}, {"x.doc", valid[0].b}, {"x.pdf", []byte("<html>fake</html>")}, {"x.txt", []byte{0xff, 0xfe}}, {"x.txt", []byte("binary\x00")}, {"x.docx", downloadZipFixture(t, "[Content_Types].xml", "xl/workbook.xml")}, {"x.xlsx", downloadZipFixture(t, "[Content_Types].xml", "word/document.xml")}, {"x.zip", downloadZipFixture(t, "../escape.txt")}, {"x.zip", downloadZipFixture(t, "C:/escape.txt")}, {"x.docx", downloadZipFixture(t, "[Content_Types].xml", "word/document.xml", "word/vbaProject.bin")}, {"empty.txt", nil}}
	for _, v := range invalid {
		if _, _, e := validateDownload(v.name, v.b); e == nil {
			t.Fatal("invalid file accepted", v.name)
		}
	}
	names := make([]string, 513)
	for i := range names {
		names[i] = strings.Repeat("a", i+1) + ".txt"
	}
	if _, _, e := validateDownload("many.zip", downloadZipFixture(t, names...)); e == nil {
		t.Fatal("unbounded archive entries accepted")
	}
}
func TestIndustryPresetIsDeterministicAndDoesNotMutateInput(t *testing.T) {
	state := model.SiteState{Version: 7, Config: model.SiteConfig{Theme: "business", SiteName: "原站点", Palette: "forest", Density: "compact", Phone: "unchanged", AnalyticsEnabled: true, AnalyticsRetention: 30, Navigation: []model.SiteLink{{Label: "原入口", URL: "/news"}}}}
	before, _ := json.Marshal(state)
	for _, def := range cmsPresetCatalog() {
		in := model.CMSPresetInput{PresetId: def.Id, SiteVersion: 7, SlugPrefix: "unit", PageKeys: []string{"about", "services", "contact"}}
		a, e := cmsBuildPreset(in, state)
		if e != nil {
			t.Fatal(e)
		}
		b, e := cmsBuildPreset(in, state)
		if e != nil || a.PlanSHA256 != b.PlanSHA256 || len(a.PlanSHA256) != 64 {
			t.Fatal("unstable plan")
		}
		if len(a.Pages) != 3 || len(a.Config.Navigation) != 4 || a.Config.Phone != "unchanged" || !a.Config.AnalyticsEnabled {
			t.Fatal("preset failed preservation rules")
		}
		after, _ := json.Marshal(state)
		if !bytes.Equal(before, after) {
			t.Fatal("preview modified source slices")
		}
	}
	for _, in := range []model.CMSPresetInput{{PresetId: "missing", SiteVersion: 7, SlugPrefix: "ok"}, {PresetId: "manufacturing", SiteVersion: 6, SlugPrefix: "ok"}, {PresetId: "manufacturing", SiteVersion: 7, SlugPrefix: "../bad"}, {PresetId: "manufacturing", SiteVersion: 7, SlugPrefix: "ok", PageKeys: []string{"about", "about"}}} {
		if _, e := cmsBuildPreset(in, state); e == nil {
			t.Fatal("invalid preset accepted")
		}
	}
	if _, e := CMSRedirectSource("/downloads/1/file"); e == nil {
		t.Fatal("download namespace can be claimed as redirect")
	}
}
