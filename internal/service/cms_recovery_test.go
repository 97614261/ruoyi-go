package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestRecoveryPartialBodyAndImages(t *testing.T) {
	if _, _, e := cmsBody("<p><br></p>", true); e != nil {
		t.Fatal(e)
	}
	if _, _, e := CMSBody("<p><br></p>"); e == nil {
		t.Fatal("ordinary save body validation weakened")
	}
	d, _, e := cmsRecoveryNormalize("PAGE", model.CMSRecoveryDraft{BodyHTML: "<p>x</p><script>alert(1)</script>"}, true)
	if e != nil || strings.Contains(d.BodyHTML, "<script") {
		t.Fatal("partial body unsafe", e)
	}
	if _, _, e = cmsRecoveryNormalize("PAGE", model.CMSRecoveryDraft{BodyHTML: `<img src="/media/1">`}, false); e == nil {
		t.Fatal("private reference accepted without permission")
	}
	if _, _, e = cmsRecoveryNormalize("PAGE", model.CMSRecoveryDraft{BodyHTML: strings.Repeat("x", (256<<10)+1)}, true); e == nil {
		t.Fatal("body limit lost")
	}
	if _, _, e = cmsRecoveryNormalize("ARTICLE", model.CMSRecoveryDraft{ProductCode: "not allowed"}, true); e == nil {
		t.Fatal("kind boundary lost")
	}
}
