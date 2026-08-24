package service

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/mojocn/base64Captcha"
)

// 验证码类型，对应 Java 版 ruoyi.captchaType。
const (
	CaptchaTypeMath = "math" // 数字计算
	CaptchaTypeChar = "char" // 字符
)

// 图片尺寸沿用 RuoYi 的 kaptcha 配置观感。
const (
	captchaHeight = 38
	captchaWidth  = 106
)

var captchaBg = &color.RGBA{R: 254, G: 254, B: 254, A: 254}

// drawCaptcha 生成验证码图片。
//
// 返回 answer（正确答案，存 Redis）和 base64 图片数据。
//
// 【注意】返回的 base64 **不带** "data:image/png;base64," 前缀。
// RuoYi 前端会自己拼 `"data:image/gif;base64," + res.img`，
// 带前缀会导致图片显示不出来。
//
// 这个文件是唯一依赖 base64Captcha 的地方，换库只需改这里。
func drawCaptcha(captchaType string) (answer, b64 string, err error) {
	// 签名对应 base64Captcha v1.3.1：
	//   NewDriverString(height, width, noiseCount, showLineOptions, length, source, bgColor, fonts)
	//   NewDriverMath(height, width, noiseCount, showLineOptions, bgColor, fonts)
	// fonts 传 nil 时 ConvertFonts() 会用内置字体。
	var driver base64Captcha.Driver
	switch captchaType {
	case CaptchaTypeChar:
		driver = base64Captcha.NewDriverString(
			captchaHeight, captchaWidth, 0,
			base64Captcha.OptionShowHollowLine,
			4, "234578abcdefhjkmnpqrstuvwxyz",
			captchaBg, nil,
		).ConvertFonts()
	default:
		driver = base64Captcha.NewDriverMath(
			captchaHeight, captchaWidth, 0,
			base64Captcha.OptionShowHollowLine,
			captchaBg, nil,
		).ConvertFonts()
	}

	_, content, answer := driver.GenerateIdQuestionAnswer()
	item, err := driver.DrawCaptcha(content)
	if err != nil {
		return "", "", fmt.Errorf("绘制验证码失败: %w", err)
	}

	encoded := item.EncodeB64string()
	// EncodeB64string 返回的是 data URI，去掉前缀只留 base64 本体
	if idx := strings.IndexByte(encoded, ','); idx >= 0 {
		encoded = encoded[idx+1:]
	}
	return answer, encoded, nil
}
