package service

import (
	"context"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
)

// parseSortPairs 解析"逗号分隔的 ID 串 + 逗号分隔的排序值串"。
//
// 部门和菜单的排序保存接口用的是同一种传输格式（Java 那边收的是
// Map<String,String>），所以共用这个解析。
func parseSortPairs(idsRaw, ordersRaw string) (map[int64]int, error) {
	idsRaw = strings.TrimSpace(idsRaw)
	if idsRaw == "" {
		return nil, errs.New("排序参数不能为空")
	}
	ids := strings.Split(idsRaw, ",")
	orders := strings.Split(strings.TrimSpace(ordersRaw), ",")
	if len(ids) != len(orders) {
		return nil, errs.New("排序参数不匹配")
	}
	if len(ids) > maxRelationIDs {
		return nil, errs.Newf("一次最多调整%d项排序", maxRelationIDs)
	}

	result := make(map[int64]int, len(ids))
	for i := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(ids[i]), 10, 64)
		if err != nil || id <= 0 {
			return nil, errs.New("排序参数格式错误")
		}
		orderNum, err := strconv.Atoi(strings.TrimSpace(orders[i]))
		if err != nil {
			return nil, errs.New("排序参数格式错误")
		}
		if _, exists := result[id]; exists {
			return nil, errs.New("排序参数包含重复ID")
		}
		result[id] = orderNum
	}
	return result, nil
}

// MenuRootID 根菜单的 parent_id。
const MenuRootID int64 = 0

// 前端约定的组件标识，对应 Java 版 UserConstants。
const (
	ComponentLayout     = "Layout"
	ComponentParentView = "ParentView"
	ComponentInnerLink  = "InnerLink"
)

// innerLinkReplacer 把内链地址转成合法路由路径。
//
// 对应 Java 版 innerLinkReplaceEach：
// http:// https:// www. 去掉，. 和 : 换成 /
// 顺序不能改 —— Replacer 在每个位置按声明顺序尝试，
// 长前缀必须排在 "." 前面，否则 "https://a.com" 会先被 "." 命中。
var innerLinkReplacer = strings.NewReplacer(
	"http://", "",
	"https://", "",
	"www.", "",
	".", "/",
	":", "/",
)

// SelectMenuTreeByUserID 查用户可见的菜单并组装成树。
//
// 超级管理员走全量查询。树根取 parent_id = 0，与 Java 版
// getChildPerms(menus, MENU_ROOT_ID) 一致。
func SelectMenuTreeByUserID(ctx context.Context, userID int64) ([]*model.SysMenu, error) {
	var (
		menus []model.SysMenu
		err   error
	)
	if userID == model.AdminUserID {
		menus, err = repository.SelectMenuTreeAll(ctx)
	} else {
		menus, err = repository.SelectMenuTreeByUserID(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	return buildMenuTree(menus, MenuRootID), nil
}

// buildMenuTree 按 parent_id 组装树。
//
// 带 visited 保护：脏数据里若存在父子成环（A 的父是 B，B 的父是 A），
// 朴素递归会栈溢出把服务打挂。Java 版没有这层保护，这里补上。
func buildMenuTree(list []model.SysMenu, rootID int64) []*model.SysMenu {
	byParent := make(map[int64][]*model.SysMenu, len(list))
	for i := range list {
		m := &list[i]
		byParent[m.ParentID] = append(byParent[m.ParentID], m)
	}

	visited := make(map[int64]bool, len(list))
	var attach func(parentID int64) []*model.SysMenu
	attach = func(parentID int64) []*model.SysMenu {
		children := byParent[parentID]
		result := make([]*model.SysMenu, 0, len(children))
		for _, c := range children {
			if visited[c.MenuID] {
				continue
			}
			visited[c.MenuID] = true
			c.Children = attach(c.MenuID)
			result = append(result, c)
		}
		return result
	}
	return attach(rootID)
}

// ListMenus 菜单管理页面的列表（含按钮），超级管理员看全部。
func ListMenus(ctx context.Context, user *model.SysUser, query model.MenuQuery) ([]model.SysMenu, error) {
	if user == nil || user.IsAdmin() {
		return repository.SelectMenuListAll(ctx, query)
	}
	return repository.SelectMenuListByUserID(ctx, user.UserID, query)
}

// GetMenu 按 ID 查菜单。
func GetMenu(ctx context.Context, menuID int64) (*model.SysMenu, error) {
	menu, err := repository.SelectMenuByID(ctx, menuID)
	if err != nil {
		return nil, err
	}
	if menu == nil {
		return nil, errs.New("菜单不存在")
	}
	return menu, nil
}

// MenuTreeSelect 菜单树选择结构。
func MenuTreeSelect(ctx context.Context, user *model.SysUser, query model.MenuQuery) ([]model.TreeSelect, error) {
	menus, err := ListMenus(ctx, user, query)
	if err != nil {
		return nil, err
	}
	return BuildMenuTreeSelect(menus), nil
}

// RoleMenuTreeSelect 菜单树 + 该角色已选中的菜单 ID。
func RoleMenuTreeSelect(ctx context.Context, user *model.SysUser, roleID int64) ([]model.TreeSelect, []int64, error) {
	if err := CheckRoleDataScope(ctx, user, roleID); err != nil {
		return nil, nil, err
	}
	role, err := repository.SelectRoleByID(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}
	checkStrictly := role != nil && role.MenuCheckStrictly

	checkedKeys, err := repository.SelectMenuIDsByRoleID(ctx, roleID, checkStrictly)
	if err != nil {
		return nil, nil, err
	}
	if checkedKeys == nil {
		checkedKeys = []int64{}
	}

	menus, err := ListMenus(ctx, user, model.MenuQuery{})
	if err != nil {
		return nil, nil, err
	}
	visible := make(map[int64]struct{}, len(menus))
	for _, menu := range menus {
		visible[menu.MenuID] = struct{}{}
	}
	filteredKeys := make([]int64, 0, len(checkedKeys))
	for _, id := range checkedKeys {
		if _, ok := visible[id]; ok {
			filteredKeys = append(filteredKeys, id)
		}
	}
	return BuildMenuTreeSelect(menus), filteredKeys, nil
}

// BuildMenuTreeSelect 把菜单列表组装成树选择结构。
//
// children 为空时不输出该键，与 Java 一致。
func BuildMenuTreeSelect(menus []model.SysMenu) []model.TreeSelect {
	byParent := make(map[int64][]model.SysMenu, len(menus))
	exists := make(map[int64]bool, len(menus))
	for _, menu := range menus {
		byParent[menu.ParentID] = append(byParent[menu.ParentID], menu)
		exists[menu.MenuID] = true
	}

	visited := make(map[int64]bool, len(menus))
	var build func(parentID int64) []model.TreeSelect
	build = func(parentID int64) []model.TreeSelect {
		children := byParent[parentID]
		if len(children) == 0 {
			return nil
		}
		nodes := make([]model.TreeSelect, 0, len(children))
		for _, menu := range children {
			if visited[menu.MenuID] {
				continue
			}
			visited[menu.MenuID] = true
			nodes = append(nodes, model.TreeSelect{
				ID:       menu.MenuID,
				Label:    html.EscapeString(menu.MenuName),
				Disabled: menu.Status == model.StatusDisable,
				Children: build(menu.MenuID),
			})
		}
		return nodes
	}

	// 父节点不在结果集里的当作根节点，避免子树整体丢失
	roots := make([]model.TreeSelect, 0)
	for _, menu := range menus {
		if exists[menu.ParentID] || visited[menu.MenuID] {
			continue
		}
		visited[menu.MenuID] = true
		roots = append(roots, model.TreeSelect{
			ID:       menu.MenuID,
			Label:    html.EscapeString(menu.MenuName),
			Disabled: menu.Status == model.StatusDisable,
			Children: build(menu.MenuID),
		})
	}
	return roots
}

// CreateMenu 新增菜单。
func CreateMenu(ctx context.Context, user *model.SysUser, menu *model.SysMenu, operator string) error {
	menuWriteMu.Lock()
	defer menuWriteMu.Unlock()

	if menu.ParentID != MenuRootID {
		if _, err := checkMenuIDsForUser(ctx, user, []int64{menu.ParentID}); err != nil {
			return err
		}
	}
	if err := checkMenuValid(ctx, menu, "新增"); err != nil {
		return err
	}
	menu.MenuID = 0
	menu.CreateBy = operator
	menu.CreateTime = types.Now()
	return repository.InsertMenu(ctx, menu)
}

// UpdateMenu 修改菜单。
func UpdateMenu(ctx context.Context, user *model.SysUser, menu *model.SysMenu, operator string) error {
	menuWriteMu.Lock()
	defer menuWriteMu.Unlock()

	if menu.MenuID == 0 {
		return errs.New("菜单ID不能为空")
	}
	if menu.ParentID == menu.MenuID {
		return errs.Newf("修改菜单'%s'失败，上级菜单不能选择自己", menu.MenuName)
	}
	ids := []int64{menu.MenuID}
	if menu.ParentID != MenuRootID {
		ids = append(ids, menu.ParentID)
	}
	if _, err := checkMenuIDsForUser(ctx, user, ids); err != nil {
		return err
	}
	if err := checkMenuValid(ctx, menu, "修改"); err != nil {
		return err
	}

	existing, err := repository.SelectMenuByID(ctx, menu.MenuID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("菜单不存在")
	}
	if menu.ParentID != MenuRootID {
		parents, err := repository.SelectMenuParentIDs(ctx)
		if err != nil {
			return err
		}
		if menuParentCreatesCycle(menu.MenuID, menu.ParentID, parents) {
			return errs.Newf("修改菜单'%s'失败，上级菜单不能选择自己的下级", menu.MenuName)
		}
	}

	menu.UpdateBy = operator
	menu.UpdateTime = types.Now()
	permissionChanged := existing.Status != menu.Status || derefString(existing.Perms) != derefString(menu.Perms)
	if !permissionChanged {
		return repository.UpdateMenu(ctx, menu)
	}

	userIDs, err := repository.SelectUserIDsByMenuID(ctx, menu.MenuID)
	if err != nil {
		return err
	}
	if err := repository.UpdateMenu(ctx, menu); err != nil {
		return err
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return RefreshOnlineUsersByID(refreshCtx, userIDs...)
}

func menuParentCreatesCycle(menuID, parentID int64, parents map[int64]int64) bool {
	seen := make(map[int64]struct{})
	for parentID != MenuRootID {
		if parentID == menuID {
			return true
		}
		if _, exists := seen[parentID]; exists {
			return true
		}
		seen[parentID] = struct{}{}
		next, exists := parents[parentID]
		if !exists {
			return false
		}
		parentID = next
	}
	return false
}

// DeleteMenu 删除菜单。
func DeleteMenu(ctx context.Context, user *model.SysUser, menuID int64) error {
	if _, err := checkMenuIDsForUser(ctx, user, []int64{menuID}); err != nil {
		return err
	}
	hasChild, err := repository.HasChildByMenuID(ctx, menuID)
	if err != nil {
		return err
	}
	if hasChild {
		return errs.New("存在子菜单,不允许删除")
	}

	assigned, err := repository.CheckMenuExistRole(ctx, menuID)
	if err != nil {
		return err
	}
	if assigned {
		return errs.New("菜单已分配,不允许删除")
	}
	return repository.DeleteMenuByID(ctx, menuID)
}

// UpdateMenuSort 保存菜单排序。
func UpdateMenuSort(ctx context.Context, user *model.SysUser, body model.MenuSortBody) error {
	sorts, err := parseSortPairs(body.MenuIDs, body.OrderNums)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(sorts))
	for id := range sorts {
		ids = append(ids, id)
	}
	if _, err := checkMenuIDsForUser(ctx, user, ids); err != nil {
		return err
	}
	return repository.UpdateMenuSort(ctx, sorts)
}

func checkMenuIDsForUser(ctx context.Context, user *model.SysUser, ids []int64) ([]int64, error) {
	normalized, err := checkMenuIDs(ctx, ids)
	if err != nil || len(normalized) == 0 || user == nil || user.IsAdmin() {
		return normalized, err
	}
	menus, err := ListMenus(ctx, user, model.MenuQuery{})
	if err != nil {
		return nil, err
	}
	visible := make([]int64, 0, len(normalized))
	requested := make(map[int64]struct{}, len(normalized))
	for _, id := range normalized {
		requested[id] = struct{}{}
	}
	for _, menu := range menus {
		if _, ok := requested[menu.MenuID]; ok {
			visible = append(visible, menu.MenuID)
		}
	}
	return normalized, ensureAllIDs(normalized, visible, "菜单")
}

// checkMenuValid 菜单名唯一 + 外链地址格式 + 路由冲突。
func checkMenuValid(ctx context.Context, menu *model.SysMenu, action string) error {
	count, err := repository.CountMenuByNameAndParent(ctx, menu.MenuName, menu.ParentID, menu.MenuID)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("%s菜单'%s'失败，菜单名称已存在", action, menu.MenuName)
	}

	// is_frame == 0 表示是外链，此时 path 必须是 http(s) 地址
	if menu.IsFrame == model.MenuIsFrameYes && !isHTTP(menu.Path) {
		return errs.Newf("%s菜单'%s'失败，地址必须以http(s)://开头", action, menu.MenuName)
	}

	conflict, err := hasRouteConflict(ctx, menu)
	if err != nil {
		return err
	}
	if conflict {
		return errs.Newf("%s菜单'%s'失败，路由名称或地址已存在", action, menu.MenuName)
	}
	return nil
}

// hasRouteConflict 路由冲突检测，对齐 Java 版 checkMenuNameUnique 之外的那段逻辑：
//
//	同级下路由地址重复 / 根目录下路由地址重复 / 路由名称全局重复
func hasRouteConflict(ctx context.Context, menu *model.SysMenu) (bool, error) {
	routeName := menu.RouteName
	if routeName == "" {
		routeName = menu.Path
	}

	candidates, err := repository.SelectMenusByPathOrRouteName(ctx, menu.Path, routeName)
	if err != nil {
		return false, err
	}
	for _, other := range candidates {
		if other.MenuID == menu.MenuID {
			continue
		}
		otherRouteName := other.RouteName
		if otherRouteName == "" {
			otherRouteName = other.Path
		}
		samePath := strings.EqualFold(menu.Path, other.Path)
		switch {
		case samePath && menu.ParentID == other.ParentID:
			return true, nil
		case samePath && menu.ParentID == MenuRootID:
			return true, nil
		case strings.EqualFold(routeName, otherRouteName):
			return true, nil
		}
	}
	return false, nil
}

// BuildRouters 把菜单树转成前端路由。
//
// 对应 Java 版 SysMenuServiceImpl.buildMenus，三个分支的顺序不能调整。
func BuildRouters(menus []*model.SysMenu) []model.RouterVo {
	routers := make([]model.RouterVo, 0, len(menus))
	for _, menu := range menus {
		router := model.RouterVo{
			Hidden:    menu.Visible == model.MenuHidden,
			Name:      routeNameOf(menu),
			Path:      safeRoutePath(routerPathOf(menu)),
			Component: componentOf(menu),
			Query:     derefString(menu.Query),
			Meta:      newMeta(html.EscapeString(menu.MenuName), menu.Icon, menu.IsCache == model.MenuCacheNo, menu.Path),
		}

		switch {
		case len(menu.Children) > 0 && menu.MenuType == model.MenuTypeDir:
			router.AlwaysShow = true
			router.Redirect = "noRedirect"
			router.Children = BuildRouters(menu.Children)

		case isMenuFrame(menu):
			// 一级菜单（非目录）：自身降级成布局容器，真实页面放进 children
			router.Meta = nil
			router.Children = []model.RouterVo{{
				Path:      safeRoutePath(menu.Path),
				Component: derefString(menu.Component),
				Name:      routeName(menu.RouteName, menu.Path),
				Query:     derefString(menu.Query),
				Meta:      newMeta(html.EscapeString(menu.MenuName), menu.Icon, menu.IsCache == model.MenuCacheNo, menu.Path),
			}}

		case menu.ParentID == MenuRootID && isInnerLink(menu):
			// 一级内链：外层挂在 "/"，内层用 InnerLink 组件套 iframe
			router.Meta = &model.MetaVo{Title: html.EscapeString(menu.MenuName), Icon: menu.Icon}
			router.Path = "/"
			routerPath := innerLinkReplacer.Replace(menu.Path)
			link := menu.Path
			router.Children = []model.RouterVo{{
				Path:      routerPath,
				Component: ComponentInnerLink,
				Name:      routeName(menu.RouteName, routerPath),
				// 这里用的是 3 参构造：link 无条件赋值，不做 http 前缀判断
				Meta: &model.MetaVo{Title: html.EscapeString(menu.MenuName), Icon: menu.Icon, Link: &link},
			}}
		}

		routers = append(routers, router)
	}
	return routers
}

func safeRoutePath(value string) string {
	return strings.NewReplacer("&", "%26", "<", "%3C", ">", "%3E", `"`, "%22", "'", "%27").Replace(value)
}

// newMeta 对应 Java 版 MetaVo(title, icon, noCache, link)。
//
// 注意 link 只在是 http(s) 地址时才赋值，否则保持 null —— 前端据此
// 判断要不要走 iframe。
func newMeta(title, icon string, noCache bool, link string) *model.MetaVo {
	meta := &model.MetaVo{Title: title, Icon: icon, NoCache: noCache}
	if isHTTP(link) {
		l := link
		meta.Link = &l
	}
	return meta
}

// routeNameOf 取路由名，一级菜单返回空串（名字由 children 承担）。
func routeNameOf(menu *model.SysMenu) string {
	if isMenuFrame(menu) {
		return ""
	}
	return routeName(menu.RouteName, menu.Path)
}

// routeName 没配路由名就用路径，首字母大写。
func routeName(name, path string) string {
	s := name
	if s == "" {
		s = path
	}
	return capitalize(s)
}

func routerPathOf(menu *model.SysMenu) string {
	path := menu.Path
	// 非一级的内链，路径需要转义
	if menu.ParentID != MenuRootID && isInnerLink(menu) {
		path = innerLinkReplacer.Replace(path)
	}
	// 一级目录：补前导斜杠
	if menu.ParentID == MenuRootID && menu.MenuType == model.MenuTypeDir && menu.IsFrame == model.MenuIsFrameNo {
		path = "/" + menu.Path
	} else if isMenuFrame(menu) {
		// 一级菜单：自身占位为根
		path = "/"
	}
	return path
}

func componentOf(menu *model.SysMenu) string {
	comp := derefString(menu.Component)
	switch {
	case comp != "" && !isMenuFrame(menu):
		return comp
	case comp == "" && menu.ParentID != MenuRootID && isInnerLink(menu):
		return ComponentInnerLink
	case comp == "" && isParentView(menu):
		return ComponentParentView
	default:
		return ComponentLayout
	}
}

// isMenuFrame 一级的普通菜单（非外链）。
func isMenuFrame(menu *model.SysMenu) bool {
	return menu.ParentID == MenuRootID &&
		menu.MenuType == model.MenuTypeMenu &&
		menu.IsFrame == model.MenuIsFrameNo
}

// isInnerLink 内链：不是外链、但路径是 http(s) 地址。
func isInnerLink(menu *model.SysMenu) bool {
	return menu.IsFrame == model.MenuIsFrameNo && isHTTP(menu.Path)
}

// isParentView 非一级的目录，用 ParentView 组件承载。
func isParentView(menu *model.SysMenu) bool {
	return menu.ParentID != MenuRootID && menu.MenuType == model.MenuTypeDir
}

func isHTTP(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
