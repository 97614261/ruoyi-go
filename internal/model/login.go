package model

// LoginBody /login 的请求体。
type LoginBody struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// Code 验证码，验证码开关关闭时可为空
	Code string `json:"code"`
	// UUID 验证码标识，对应 Redis 的 captcha_codes:<uuid>
	UUID string `json:"uuid"`
}

// RegisterBody /register 的请求体。
//
// 长度校验放在 service 里做，因为要返回 Java 那套具体文案
// （"账户长度必须在2到20个字符之间"），binding 的通用提示对不上。
type RegisterBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
	UUID     string `json:"uuid"`
}

// LoginUser 存放在 Redis login_tokens:<uuid> 里的会话对象。
//
// 【注意】这份数据是权限判断的依据，不是缓存优化。
// token 里只有 uuid，用户是谁、有什么权限全靠这里。
//
// 序列化用 JSON。因为 SysUser.MarshalJSON 会剔除 password，
// 反序列化回来的 User.Password 是空串 —— 这是预期行为，
// 需要校验密码的场景（如修改密码）应重新查库。
type LoginUser struct {
	// Token 即 login_user_key，随机 uuid
	Token  string `json:"token"`
	UserID int64  `json:"userId"`
	DeptID *int64 `json:"deptId"`
	// LoginTime / ExpireTime 均为 Unix 毫秒
	LoginTime  int64 `json:"loginTime"`
	ExpireTime int64 `json:"expireTime"`

	IPAddr        string `json:"ipaddr"`
	LoginLocation string `json:"loginLocation"`
	Browser       string `json:"browser"`
	OS            string `json:"os"`

	// Permissions 权限标识集合，超级管理员为 ["*:*:*"]
	Permissions []string `json:"permissions"`
	User        *SysUser `json:"user"`
}

// AllPermission 超级管理员的通配权限标识。
const AllPermission = "*:*:*"

// AdminRoleKey 超级管理员角色标识。
const AdminRoleKey = "admin"

// HasPermission 判断是否拥有某个权限标识。
func (l *LoginUser) HasPermission(perm string) bool {
	if l == nil || perm == "" {
		return false
	}
	for _, p := range l.Permissions {
		if p == AllPermission || p == perm {
			return true
		}
	}
	return false
}
