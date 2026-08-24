// Package jwtx JWT 签发与解析。
//
// 【设计要点】token 里不承载用户数据，只有一个随机 uuid 和用户名。
// 真实会话在 Redis 的 login_tokens:<uuid>，有效期也由 Redis TTL 控制——
// token 本身不含 exp 声明，永不自然过期（与 Java 版 TokenService 一致）。
//
// 这样设计是为了保留服务端会话控制能力：在线用户列表、强制下线、
// 改权限后即时生效、改密码后使旧会话失效。不要为了省一次 Redis 查询
// 把用户信息塞进 claims，那是删功能不是优化。
package jwtx

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// claim 名称，与 Java 版 Constants.java 对齐。
const (
	ClaimLoginUserKey = "login_user_key"
	ClaimUserName     = "sub"
	// TokenPrefix Authorization 头的前缀，注意尾部有空格。
	TokenPrefix = "Bearer "
)

// ErrInvalidToken token 无效或签名不匹配。
var ErrInvalidToken = errors.New("令牌无效")

// Claims 从 token 中解析出的内容。
//
// 注意这里只有两个字段。用户 ID、角色、权限都不在 token 里，
// 必须拿 LoginUserKey 去 Redis 查。
type Claims struct {
	LoginUserKey string
	UserName     string
}

// Signer 使用 HS512 签发与校验，与 Java 版保持一致。
//
// 注意：本实现与 jjwt 0.9.1 的密钥处理方式不同（jjwt 会把密钥串当作
// base64 解码），因此两版签出的 token **不能互相识别**。切换后端需要
// 用户重新登录一次，这是可接受的一次性成本。
type Signer struct {
	secret []byte
}

// NewSigner 创建签发器。
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// Sign 签发 token。loginUserKey 应为随机 uuid。
func (s *Signer) Sign(loginUserKey, userName string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		ClaimLoginUserKey: loginUserKey,
		ClaimUserName:     userName,
	})
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("签发令牌失败: %w", err)
	}
	return signed, nil
}

// Parse 校验签名并取出 claims。
func (s *Signer) Parse(tokenStr string) (*Claims, error) {
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: 签名算法不匹配 %v", ErrInvalidToken, t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}

	key, _ := mc[ClaimLoginUserKey].(string)
	if key == "" {
		return nil, fmt.Errorf("%w: 缺少 %s", ErrInvalidToken, ClaimLoginUserKey)
	}
	name, _ := mc[ClaimUserName].(string)
	return &Claims{LoginUserKey: key, UserName: name}, nil
}

// StripPrefix 去掉 Authorization 头的 "Bearer " 前缀，没有前缀则原样返回。
func StripPrefix(header string) string {
	if len(header) > len(TokenPrefix) && header[:len(TokenPrefix)] == TokenPrefix {
		return header[len(TokenPrefix):]
	}
	return header
}
