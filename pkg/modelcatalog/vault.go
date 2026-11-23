package modelcatalog

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
)

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
var keyName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type Vault struct {
	active string
	keys   map[string][]byte
	mac    []byte
	envs   map[string]bool
}

type keyring struct {
	ActiveKeyID    string            `json:"activeKeyId"`
	Keys           map[string]string `json:"keys"`
	FingerprintKey string            `json:"fingerprintKey"`
}

func NewVault(path, jwt string, allowed []string) (*Vault, error) {
	v := &Vault{envs: map[string]bool{}, keys: map[string][]byte{}}
	for _, name := range allowed {
		if !envName.MatchString(name) || v.envs[name] {
			return nil, errors.New("模型凭据环境变量白名单无效")
		}
		v.envs[name] = true
	}
	if path == "" {
		return v, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("模型主密钥文件不可读")
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > 32768 {
		return nil, errors.New("模型主密钥文件必须为不超过 32 KiB 的普通文件")
	}
	var ring keyring
	d := json.NewDecoder(io.LimitReader(f, 32769))
	d.DisallowUnknownFields()
	if d.Decode(&ring) != nil || d.Decode(new(any)) != io.EOF || len(ring.Keys) < 1 || len(ring.Keys) > 8 || !keyName.MatchString(ring.ActiveKeyID) {
		return nil, errors.New("模型主密钥文件格式无效")
	}
	decode := func(s string) ([]byte, error) {
		b, err := base64.StdEncoding.Strict().DecodeString(s)
		if err != nil || len(b) != 32 || s == jwt || bytes.Equal(b, []byte(jwt)) {
			return nil, errors.New("模型密钥必须为独立的 32 字节随机值")
		}
		return b, nil
	}
	if v.mac, err = decode(ring.FingerprintKey); err != nil {
		return nil, err
	}
	for id, value := range ring.Keys {
		if !keyName.MatchString(id) {
			return nil, errors.New("模型密钥标识无效")
		}
		key, err := decode(value)
		if err != nil || bytes.Equal(key, v.mac) {
			return nil, errors.New("加密密钥与幂等指纹密钥必须独立")
		}
		v.keys[id] = key
	}
	if v.keys[ring.ActiveKeyID] == nil {
		return nil, errors.New("模型活动主密钥缺失")
	}
	v.active = ring.ActiveKeyID
	return v, nil
}

func (v *Vault) Ready() bool { return v != nil && v.active != "" && len(v.mac) == 32 }

func (v *Vault) Fingerprint(action string, value any) (string, error) {
	if !v.Ready() {
		return "", errors.New("模型主密钥尚未配置，不能执行管理写操作")
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) > 128<<10 {
		return "", errors.New("模型管理请求无效")
	}
	m := hmac.New(sha256.New, v.mac)
	m.Write([]byte("pod-admin-action:v1\x00" + action + "\x00"))
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil)), nil
}

func (v *Vault) Seal(provider, version int64, secret string) (string, []byte, []byte, error) {
	if !v.Ready() {
		return "", nil, nil, errors.New("模型主密钥尚未配置")
	}
	a, err := v.aead(v.active)
	if err != nil {
		return "", nil, nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", nil, nil, errors.New("模型凭据加密失败")
	}
	return v.active, nonce, a.Seal(nil, nonce, []byte(secret), credentialAAD(provider, version)), nil
}

func (v *Vault) Open(provider, version int64, keyID string, nonce, ciphertext []byte) (string, error) {
	a, err := v.aead(keyID)
	if err != nil || len(nonce) != a.NonceSize() {
		return "", errors.New("模型凭据无法解密")
	}
	b, err := a.Open(nil, nonce, ciphertext, credentialAAD(provider, version))
	if err != nil {
		return "", errors.New("模型凭据无法解密")
	}
	return string(b), nil
}

func (v *Vault) aead(id string) (cipher.AEAD, error) {
	if v == nil || len(v.keys[id]) != 32 {
		return nil, errors.New("模型主密钥版本不可用")
	}
	b, err := aes.NewCipher(v.keys[id])
	if err != nil {
		return nil, errors.New("模型主密钥不可用")
	}
	return cipher.NewGCM(b)
}
func credentialAAD(provider, version int64) []byte {
	return []byte(fmt.Sprintf("pod-provider-credential:v1:%d:%d", provider, version))
}

func (v *Vault) EnvAllowed(name string) bool { return v != nil && v.envs[name] }
func (v *Vault) ResolveEnv(name string) (string, error) {
	if !v.EnvAllowed(name) {
		return "", errors.New("凭据环境变量不在部署白名单内")
	}
	s := os.Getenv(name)
	if !ValidSecret(s) {
		return "", errors.New("凭据环境变量未设置或格式无效")
	}
	return s, nil
}
func ValidSecret(s string) bool {
	if len(s) < 8 || len(s) > 512 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
