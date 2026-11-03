// Generated service, specialized for bounded anonymous submission and one-use proofs.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"github.com/redis/go-redis/v9"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
	"strings"
	"time"
	"unicode"
)

func inquiryIP(ip string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(ip))) }
func inquiryLimit(ctx context.Context, key string, n int, ttl time.Duration) error {
	v, e := redisx.RateLimit(ctx, key, n, ttl)
	if e != nil {
		return errs.New("留言暂不可用，请稍后再试")
	}
	if v > int64(n) {
		return errs.New("提交过于频繁，请稍后再试")
	}
	return nil
}
func CMSInquiryProof(ctx context.Context, ip string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if e := inquiryLimit(ctx, "portal_inquiry:form:"+inquiryIP(ip), 30, time.Hour); e != nil {
		return "", e
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	token := hex.EncodeToString(b)
	value := fmt.Sprintf("%d:%s", time.Now().Unix(), inquiryIP(ip))
	if e := redisx.C().Set(ctx, "portal_inquiry:proof:"+token, value, 10*time.Minute).Err(); e != nil {
		return "", errs.New("留言暂不可用，请稍后再试")
	}
	return token, nil
}

var inquiryConsume = redis.NewScript(`local value=redis.call('GET',KEYS[1]);if not value then return 0 end;local colon=string.find(value,':');if not colon then return 0 end;if string.sub(value,colon+1)~=ARGV[1] then return 0 end;if tonumber(ARGV[2])-tonumber(string.sub(value,1,colon-1))<2 then return 0 end;redis.call('DEL',KEYS[1]);return 1`)

func CMSSubmitInquiry(ctx context.Context, in model.CMSInquiryInput, ip, cookie string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if len(cookie) != 64 || subtle.ConstantTimeCompare([]byte(cookie), []byte(in.Token)) != 1 || in.Website != "" {
		return errs.New("提交校验失败，请重新打开留言页")
	}
	for _, s := range []string{in.Name, in.Contact, in.Message} {
		if strings.TrimSpace(s) == "" || strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' }) {
			return errs.New("请填写有效内容")
		}
	}
	if e := inquiryLimit(ctx, "portal_inquiry:submit:"+inquiryIP(ip), 5, time.Hour); e != nil {
		return e
	}
	if e := inquiryLimit(ctx, "portal_inquiry:global", 1000, 24*time.Hour); e != nil {
		return e
	}
	ok, e := inquiryConsume.Run(ctx, redisx.C(), []string{"portal_inquiry:proof:" + in.Token}, inquiryIP(ip), time.Now().Unix()).Int()
	if e != nil || ok != 1 {
		return errs.New("页面已过期或提交太快，请重新打开留言页并稍候")
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Contact = strings.TrimSpace(in.Contact)
	in.Message = strings.TrimSpace(in.Message)
	return repository.CMSInsertInquiry(ctx, in)
}
func CMSInquiryPage(ctx context.Context, q model.CMSInquiryQuery, pg page.Query) ([]model.CMSInquirySummary, int64, error) {
	return repository.CMSInquiryPage(ctx, q, pg)
}
func CMSInquiryDetail(ctx context.Context, id int64) (*model.CMSInquiry, error) {
	return repository.CMSInquiryDetail(ctx, id)
}
func CMSUpdateInquiry(ctx context.Context, id int64, in model.CMSInquiryUpdate, operator string, erase bool) error {
	return repository.CMSUpdateInquiry(ctx, id, in, operator, erase)
}
