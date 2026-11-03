package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const CMSDownloadMaxBytes int64 = 20 << 20

var downloadStorageKey = regexp.MustCompile(`^dl-[a-f0-9-]{36}\.(pdf|docx|xlsx|txt|zip)$`)
var downloadMIME = map[string]string{"pdf": "application/pdf", "docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "txt": "text/plain; charset=utf-8", "zip": "application/zip"}

func validateDownload(name string, b []byte) (string, string, error) {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 160 || strings.ContainsAny(name, "<>/\\") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", "", errs.New("文件名需1到160字，不含尖括号、路径或控制字符")
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	mime, ok := downloadMIME[ext]
	if !ok {
		return "", "", errs.New("只支持PDF、DOCX、XLSX、TXT和ZIP，不支持可执行文件或旧版DOC/XLS")
	}
	if len(b) == 0 || int64(len(b)) > CMSDownloadMaxBytes {
		return "", "", errs.New("资料附件必须非空且不超过20MiB")
	}
	switch ext {
	case "pdf":
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			return "", "", errs.New("PDF内容与扩展名不匹配")
		}
	case "txt":
		if !utf8.Valid(b) || strings.IndexFunc(string(b), func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) >= 0 {
			return "", "", errs.New("TXT须为不含二进制控制字符的UTF-8文本")
		}
	default:
		z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if e != nil || len(z.File) == 0 || len(z.File) > 512 {
			return "", "", errs.New("压缩文件损坏、为空或超过512个条目")
		}
		names := map[string]bool{}
		var total uint64
		for _, f := range z.File {
			n := strings.ReplaceAll(f.Name, "\\", "/")
			clean := path.Clean(n)
			if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") || f.Flags&1 != 0 {
				return "", "", errs.New("不支持含越界路径或加密条目的压缩文件")
			}
			if f.UncompressedSize64 > 128<<20 || total > (128<<20)-f.UncompressedSize64 {
				return "", "", errs.New("解压后声明总量不能超过128MiB")
			}
			total += f.UncompressedSize64
			names[n] = true
			if strings.HasSuffix(strings.ToLower(n), "vbaproject.bin") {
				return "", "", errs.New("不支持含宏的Office文档")
			}
		}
		if ext == "docx" && (!names["[Content_Types].xml"] || !names["word/document.xml"]) {
			return "", "", errs.New("DOCX内容与扩展名不匹配")
		}
		if ext == "xlsx" && (!names["[Content_Types].xml"] || !names["xl/workbook.xml"]) {
			return "", "", errs.New("XLSX内容与扩展名不匹配")
		}
	}
	return ext, mime, nil
}
func CMSUploadDownload(ctx context.Context, src io.Reader, name string) (model.CMSDownloadFile, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var row model.CMSDownloadFile
	b, e := io.ReadAll(io.LimitReader(src, CMSDownloadMaxBytes+1))
	if e != nil {
		return row, errs.New("读取资料附件失败")
	}
	if e = ctx.Err(); e != nil {
		return row, e
	}
	ext, mime, e := validateDownload(name, b)
	if e != nil {
		return row, e
	}
	if cmsAssetRoot == "" {
		return row, fmt.Errorf("private download storage unavailable")
	}
	key := "dl-" + uuid.NewString() + "." + ext
	p := filepath.Join(cmsAssetRoot, key)
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return row, e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		_ = os.Remove(p)
		return row, e
	}
	row = model.CMSDownloadFile{StorageKey: key, Name: name, Extension: ext, MimeType: mime, ByteSize: int64(len(b)), SHA256: fmt.Sprintf("%x", sha256.Sum256(b))}
	row, e = repository.CMSCreateDownloadFile(ctx, row)
	if e != nil {
		_ = os.Remove(p)
	}
	return row, e
}
func cmsOpenDownload(row model.CMSDownloadFile) (*os.File, error) {
	if !downloadStorageKey.MatchString(row.StorageKey) || cmsAssetRoot == "" {
		return nil, fmt.Errorf("invalid download storage key")
	}
	p := filepath.Join(cmsAssetRoot, row.StorageKey)
	info, e := os.Lstat(p)
	if e != nil || !info.Mode().IsRegular() || info.Size() != row.ByteSize {
		return nil, errs.New("资料文件缺失或异常，请管理员检查")
	}
	f, e := os.Open(p)
	return f, e
}
func CMSOpenDownloadFile(ctx context.Context, id int64, public bool) (*os.File, model.CMSDownloadFile, error) {
	var row model.CMSDownloadFile
	var e error
	if public {
		row, e = repository.PortalDownloadFile(ctx, id)
	} else {
		row, e = repository.CMSPrivateDownloadFile(ctx, id)
	}
	if e != nil {
		return nil, row, e
	}
	f, e := cmsOpenDownload(row)
	return f, row, e
}
func CMSDeleteDownloadFile(ctx context.Context, id, version int64) error {
	row, e := repository.CMSDeleteDownloadFile(ctx, id, version)
	if e != nil {
		return e
	}
	if !downloadStorageKey.MatchString(row.StorageKey) || cmsAssetRoot == "" {
		return fmt.Errorf("invalid download storage key")
	}
	if e = os.Remove(filepath.Join(cmsAssetRoot, row.StorageKey)); e != nil && !os.IsNotExist(e) {
		slog.Warn("资料附件已停用，物理文件待清理", "fileId", id)
	}
	return nil
}
func CMSDownloadCategories(ctx context.Context, q model.CMSDownloadCategoryQuery, pg page.Query, public bool) ([]model.CMSDownloadCategory, int64, error) {
	return repository.CMSDownloadCategories(ctx, q, pg, public)
}
func CMSSaveDownloadCategory(ctx context.Context, in model.CMSDownloadCategoryInput) (int64, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.CategoryId > 0 && in.Version < 1 {
		return 0, errs.New("请提交分类当前版本")
	}
	return repository.CMSSaveDownloadCategory(ctx, in)
}
func CMSRemoveDownloadCategory(ctx context.Context, id, version int64) error {
	return repository.CMSRemoveDownloadCategory(ctx, id, version)
}
func CMSDownloadFiles(ctx context.Context, in model.CMSDownloadFileQuery, pg page.Query) ([]model.CMSDownloadFile, int64, error) {
	return repository.CMSDownloadFiles(ctx, in, pg)
}
func CMSDownloadPage(ctx context.Context, in model.CMSDownloadQuery, pg page.Query) ([]model.CMSDownloadView, int64, error) {
	return repository.CMSDownloadPage(ctx, in, pg)
}
func CMSDownloadDetail(ctx context.Context, id int64) (model.CMSDownloadDetail, error) {
	return repository.CMSDownloadDetail(ctx, id)
}
func CMSSaveDownload(ctx context.Context, in model.CMSDownloadInput) (int64, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.DownloadId > 0 && in.Version < 1 {
		return 0, errs.New("请提交资料当前版本")
	}
	return repository.CMSSaveDownload(ctx, in)
}
func CMSChangeDownload(ctx context.Context, id, version int64, action string) error {
	return repository.CMSChangeDownload(ctx, id, version, action)
}
func CMSDownloadHistory(ctx context.Context, id int64, pg page.Query) ([]model.CMSDownloadRevision, int64, error) {
	return repository.CMSDownloadHistory(ctx, id, pg)
}
func PortalDownloadPage(ctx context.Context, in model.CMSDownloadQuery, offset, limit int) ([]model.PortalDownload, int64, error) {
	return repository.PortalDownloadPage(ctx, in, offset, limit)
}
