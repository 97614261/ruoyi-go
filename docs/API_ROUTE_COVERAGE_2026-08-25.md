# 非代码生成器接口路由与对拍覆盖清单

生成命令：`go run ./cmd/routeaudit -java-root ../RuoYi-Vue-master`

## 汇总

| 指标 | 数量 |
|---|---:|
| Go 路由 | 131 |
| Java 路由（排除代码生成器、Swagger 内存演示接口和 dev/test Profile） | 130 |
| 方法 + 结构化路径匹配 | 130 |
| 权限标识匹配 | 130 |
| 权限标识差异 | 0 |
| 已纳入自动双端探针的匹配路由 | 73/130（56.15%） |

> “已纳入探针”只表示有自动执行入口；是否通过以对应实跑日志为准。
> 路径参数名会统一为 `{}` 比较，避免 `{id}` / `{userId}` 这种非契约差异。

## 完整清单

| 方法与路径 | Go | Java | 权限 | 自动双端证据 |
|---|---|---|---|---|
| `DELETE /monitor/cache/clearCacheAll` | 有（`CacheClearAll`） | 有（`clearCacheAll`） | `monitor:cache:list`（一致） | 未纳入 |
| `DELETE /monitor/cache/clearCacheKey/{}` | 有（`CacheClearKey`） | 有（`clearCacheKey`） | `monitor:cache:list`（一致） | 错误场景 |
| `DELETE /monitor/cache/clearCacheName/{}` | 有（`CacheClearName`） | 有（`clearCacheName`） | `monitor:cache:list`（一致） | 错误场景 |
| `DELETE /monitor/job/{}` | 有（`JobRemove`） | 有（`remove`） | `monitor:job:remove`（一致） | CRUD |
| `DELETE /monitor/jobLog/clean` | 有（`JobLogClean`） | 有（`clean`） | `monitor:job:remove`（一致） | 未纳入 |
| `DELETE /monitor/jobLog/{}` | 有（`JobLogRemove`） | 有（`remove`） | `monitor:job:remove`（一致） | 未纳入 |
| `DELETE /monitor/logininfor/clean` | 有（`LogininforClean`） | 有（`clean`） | `monitor:logininfor:remove`（一致） | 未纳入 |
| `DELETE /monitor/logininfor/{}` | 有（`LogininforRemove`） | 有（`remove`） | `monitor:logininfor:remove`（一致） | 未纳入 |
| `DELETE /monitor/online/{}` | 有（`OnlineForceLogout`） | 有（`forceLogout`） | `monitor:online:forceLogout`（一致） | 未纳入 |
| `DELETE /monitor/operlog/clean` | 有（`OperLogClean`） | 有（`clean`） | `monitor:operlog:remove`（一致） | 未纳入 |
| `DELETE /monitor/operlog/{}` | 有（`OperLogRemove`） | 有（`remove`） | `monitor:operlog:remove`（一致） | 未纳入 |
| `DELETE /system/config/refreshCache` | 有（`ConfigRefreshCache`） | 有（`refreshCache`） | `system:config:remove`（一致） | 未纳入 |
| `DELETE /system/config/{}` | 有（`ConfigRemove`） | 有（`remove`） | `system:config:remove`（一致） | CRUD |
| `DELETE /system/dept/{}` | 有（`DeptRemove`） | 有（`remove`） | `system:dept:remove`（一致） | CRUD |
| `DELETE /system/dict/data/{}` | 有（`DictDataRemove`） | 有（`remove`） | `system:dict:remove`（一致） | CRUD |
| `DELETE /system/dict/type/refreshCache` | 有（`DictTypeRefreshCache`） | 有（`refreshCache`） | `system:dict:remove`（一致） | 未纳入 |
| `DELETE /system/dict/type/{}` | 有（`DictTypeRemove`） | 有（`remove`） | `system:dict:remove`（一致） | CRUD |
| `DELETE /system/menu/{}` | 有（`MenuRemove`） | 有（`remove`） | `system:menu:remove`（一致） | CRUD |
| `DELETE /system/notice/{}` | 有（`NoticeRemove`） | 有（`remove`） | `system:notice:remove`（一致） | CRUD |
| `DELETE /system/post/{}` | 有（`PostRemove`） | 有（`remove`） | `system:post:remove`（一致） | CRUD |
| `DELETE /system/role/{}` | 有（`RoleRemove`） | 有（`remove`） | `system:role:remove`（一致） | CRUD |
| `DELETE /system/user/{}` | 有（`UserRemove`） | 有（`remove`） | `system:user:remove`（一致） | CRUD |
| `GET /captchaImage` | 有（`Captcha`） | 有（`getCode`） | 无（一致） | 核心 GET |
| `GET /common/download` | 有（`CommonDownload`） | 有（`fileDownload`） | 无（一致） | 未纳入 |
| `GET /common/download/resource` | 有（`CommonDownloadResource`） | 有（`resourceDownload`） | 无（一致） | 未纳入 |
| `GET /getInfo` | 有（`GetInfo`） | 有（`getInfo`） | 无（一致） | 核心 GET |
| `GET /getRouters` | 有（`GetRouters`） | 有（`getRouters`） | 无（一致） | 核心 GET |
| `GET /health` | 有（`Health`） | 缺失 | — | 未纳入 |
| `GET /monitor/cache` | 有（`CacheInfo`） | 有（`getInfo`） | `monitor:cache:list`（一致） | 未纳入 |
| `GET /monitor/cache/getKeys/{}` | 有（`CacheKeys`） | 有（`getCacheKeys`） | `monitor:cache:list`（一致） | 未纳入 |
| `GET /monitor/cache/getNames` | 有（`CacheNames`） | 有（`cache`） | `monitor:cache:list`（一致） | 未纳入 |
| `GET /monitor/cache/getValue/{}/{}` | 有（`CacheValue`） | 有（`getCacheValue`） | `monitor:cache:list`（一致） | 未纳入 |
| `GET /monitor/job/list` | 有（`JobList`） | 有（`list`） | `monitor:job:list`（一致） | 核心 GET |
| `GET /monitor/job/{}` | 有（`JobGet`） | 有（`getInfo`） | `monitor:job:query`（一致） | 核心 GET、CRUD |
| `GET /monitor/jobLog/list` | 有（`JobLogList`） | 有（`list`） | `monitor:job:list`（一致） | 核心 GET |
| `GET /monitor/jobLog/{}` | 有（`JobLogGet`） | 有（`getInfo`） | `monitor:job:query`（一致） | 未纳入 |
| `GET /monitor/logininfor/list` | 有（`LogininforList`） | 有（`list`） | `monitor:logininfor:list`（一致） | 未纳入 |
| `GET /monitor/logininfor/unlock/{}` | 有（`LogininforUnlock`） | 有（`unlock`） | `monitor:logininfor:unlock`（一致） | 未纳入 |
| `GET /monitor/online/list` | 有（`OnlineList`） | 有（`list`） | `monitor:online:list`（一致） | 未纳入 |
| `GET /monitor/operlog/list` | 有（`OperLogList`） | 有（`list`） | `monitor:operlog:list`（一致） | 未纳入 |
| `GET /monitor/server` | 有（`ServerInfo`） | 有（`getInfo`） | `monitor:server:list`（一致） | 未纳入 |
| `GET /system/config/configKey/{}` | 有（`ConfigGetByKey`） | 有（`getConfigKey`） | 无（一致） | 核心 GET |
| `GET /system/config/list` | 有（`ConfigList`） | 有（`list`） | `system:config:list`（一致） | 核心 GET |
| `GET /system/config/{}` | 有（`ConfigGet`） | 有（`getInfo`） | `system:config:query`（一致） | 核心 GET、CRUD |
| `GET /system/dept/list` | 有（`DeptList`） | 有（`list`） | `system:dept:list`（一致） | 核心 GET、数据/功能权限 |
| `GET /system/dept/list/exclude/{}` | 有（`DeptExcludeChild`） | 有（`excludeChild`） | `system:dept:list`（一致） | 未纳入 |
| `GET /system/dept/{}` | 有（`DeptGet`） | 有（`getInfo`） | `system:dept:query`（一致） | 核心 GET、CRUD |
| `GET /system/dict/data/list` | 有（`DictDataList`） | 有（`list`） | `system:dict:list`（一致） | 核心 GET |
| `GET /system/dict/data/type/{}` | 有（`DictDataByType`） | 有（`dictType`） | 无（一致） | 核心 GET |
| `GET /system/dict/data/{}` | 有（`DictDataGet`） | 有（`getInfo`） | `system:dict:query`（一致） | 核心 GET、CRUD |
| `GET /system/dict/type/list` | 有（`DictTypeList`） | 有（`list`） | `system:dict:list`（一致） | 核心 GET |
| `GET /system/dict/type/optionselect` | 有（`DictTypeOptionSelect`） | 有（`optionselect`） | 无（一致） | 核心 GET |
| `GET /system/dict/type/{}` | 有（`DictTypeGet`） | 有（`getInfo`） | `system:dict:query`（一致） | 核心 GET、CRUD |
| `GET /system/menu/list` | 有（`MenuList`） | 有（`list`） | `system:menu:list`（一致） | 核心 GET |
| `GET /system/menu/roleMenuTreeselect/{}` | 有（`MenuRoleTreeSelect`） | 有（`roleMenuTreeselect`） | 无（一致） | 核心 GET |
| `GET /system/menu/treeselect` | 有（`MenuTreeSelect`） | 有（`treeselect`） | 无（一致） | 核心 GET |
| `GET /system/menu/{}` | 有（`MenuGet`） | 有（`getInfo`） | `system:menu:query`（一致） | 核心 GET、CRUD |
| `GET /system/notice/list` | 有（`NoticeList`） | 有（`list`） | `system:notice:list`（一致） | 核心 GET |
| `GET /system/notice/listTop` | 有（`NoticeListTop`） | 有（`listTop`） | 无（一致） | 核心 GET |
| `GET /system/notice/readUsers/list` | 有（`NoticeReadUsers`） | 有（`readUsersList`） | `system:notice:list`（一致） | 未纳入 |
| `GET /system/notice/{}` | 有（`NoticeGet`） | 有（`getInfo`） | 无（一致） | 核心 GET、CRUD |
| `GET /system/post/list` | 有（`PostList`） | 有（`list`） | `system:post:list`（一致） | 核心 GET |
| `GET /system/post/optionselect` | 有（`PostOptionSelect`） | 有（`optionselect`） | 无（一致） | 核心 GET |
| `GET /system/post/{}` | 有（`PostGet`） | 有（`getInfo`） | `system:post:query`（一致） | 核心 GET、CRUD |
| `GET /system/role/authUser/allocatedList` | 有（`RoleAuthUserAllocated`） | 有（`allocatedList`） | `system:role:list`（一致） | 未纳入 |
| `GET /system/role/authUser/unallocatedList` | 有（`RoleAuthUserUnallocated`） | 有（`unallocatedList`） | `system:role:list`（一致） | 未纳入 |
| `GET /system/role/deptTree/{}` | 有（`RoleDeptTree`） | 有（`deptTree`） | `system:role:query`（一致） | 核心 GET |
| `GET /system/role/list` | 有（`RoleList`） | 有（`list`） | `system:role:list`（一致） | 核心 GET、数据/功能权限 |
| `GET /system/role/optionselect` | 有（`RoleOptionSelect`） | 有（`optionselect`） | `system:role:query`（一致） | 核心 GET |
| `GET /system/role/{}` | 有（`RoleGet`） | 有（`getInfo`） | `system:role:query`（一致） | 核心 GET、CRUD |
| `GET /system/user` | 有（`UserGet`） | 有（`getInfo`） | `system:user:query`（一致） | 核心 GET |
| `GET /system/user/authRole/{}` | 有（`UserAuthRoleGet`） | 有（`authRole`） | `system:user:query`（一致） | 未纳入 |
| `GET /system/user/deptTree` | 有（`UserDeptTree`） | 有（`deptTree`） | `system:user:list`（一致） | 核心 GET |
| `GET /system/user/list` | 有（`UserList`） | 有（`list`） | `system:user:list`（一致） | 核心 GET、数据/功能权限 |
| `GET /system/user/profile` | 有（`ProfileGet`） | 有（`profile`） | 无（一致） | 未纳入 |
| `GET /system/user/{}` | 有（`UserGet`） | 有（`getInfo`） | `system:user:query`（一致） | 核心 GET、CRUD |
| `POST /common/upload` | 有（`CommonUpload`） | 有（`uploadFile`） | 无（一致） | 文件、错误场景 |
| `POST /common/uploads` | 有（`CommonUploads`） | 有（`uploadFiles`） | 无（一致） | 文件、错误场景 |
| `POST /login` | 有（`Login`） | 有（`login`） | 无（一致） | 未纳入 |
| `POST /logout` | 有（`Logout`） | 有（`LogoutSuccessHandlerImpl`） | 无（一致） | 未纳入 |
| `POST /monitor/job` | 有（`JobAdd`） | 有（`add`） | `monitor:job:add`（一致） | CRUD、字段校验 |
| `POST /monitor/job/export` | 有（`JobExport`） | 有（`export`） | `monitor:job:export`（一致） | 未纳入 |
| `POST /monitor/jobLog/export` | 有（`JobLogExport`） | 有（`export`） | `monitor:job:export`（一致） | 未纳入 |
| `POST /monitor/logininfor/export` | 有（`LogininforExport`） | 有（`export`） | `monitor:logininfor:export`（一致） | 未纳入 |
| `POST /monitor/operlog/export` | 有（`OperLogExport`） | 有（`export`） | `monitor:operlog:export`（一致） | 未纳入 |
| `POST /register` | 有（`Register`） | 有（`register`） | 无（一致） | 错误场景 |
| `POST /system/config` | 有（`ConfigAdd`） | 有（`add`） | `system:config:add`（一致） | CRUD、字段校验 |
| `POST /system/config/export` | 有（`ConfigExport`） | 有（`export`） | `system:config:export`（一致） | 未纳入 |
| `POST /system/dept` | 有（`DeptAdd`） | 有（`add`） | `system:dept:add`（一致） | CRUD、字段校验 |
| `POST /system/dict/data` | 有（`DictDataAdd`） | 有（`add`） | `system:dict:add`（一致） | CRUD、字段校验 |
| `POST /system/dict/data/export` | 有（`DictDataExport`） | 有（`export`） | `system:dict:export`（一致） | 未纳入 |
| `POST /system/dict/type` | 有（`DictTypeAdd`） | 有（`add`） | `system:dict:add`（一致） | CRUD、字段校验 |
| `POST /system/dict/type/export` | 有（`DictTypeExport`） | 有（`export`） | `system:dict:export`（一致） | 未纳入 |
| `POST /system/menu` | 有（`MenuAdd`） | 有（`add`） | `system:menu:add`（一致） | CRUD、字段校验 |
| `POST /system/notice` | 有（`NoticeAdd`） | 有（`add`） | `system:notice:add`（一致） | CRUD、字段校验 |
| `POST /system/notice/markRead` | 有（`NoticeMarkRead`） | 有（`markRead`） | 无（一致） | 未纳入 |
| `POST /system/notice/markReadAll` | 有（`NoticeMarkReadAll`） | 有（`markReadAll`） | 无（一致） | 未纳入 |
| `POST /system/post` | 有（`PostAdd`） | 有（`add`） | `system:post:add`（一致） | CRUD、字段校验 |
| `POST /system/post/export` | 有（`PostExport`） | 有（`export`） | `system:post:export`（一致） | 未纳入 |
| `POST /system/role` | 有（`RoleAdd`） | 有（`add`） | `system:role:add`（一致） | CRUD、字段校验 |
| `POST /system/role/export` | 有（`RoleExport`） | 有（`export`） | `system:role:export`（一致） | 未纳入 |
| `POST /system/user` | 有（`UserAdd`） | 有（`add`） | `system:user:add`（一致） | CRUD、字段校验 |
| `POST /system/user/export` | 有（`UserExport`） | 有（`export`） | `system:user:export`（一致） | 未纳入 |
| `POST /system/user/importData` | 有（`UserImportData`） | 有（`importData`） | `system:user:import`（一致） | 文件、错误场景 |
| `POST /system/user/importTemplate` | 有（`UserImportTemplate`） | 有（`importTemplate`） | 无（一致） | 文件 |
| `POST /system/user/profile/avatar` | 有（`ProfileAvatar`） | 有（`avatar`） | 无（一致） | 错误场景 |
| `POST /unlockscreen` | 有（`UnlockScreen`） | 有（`unlockScreen`） | 无（一致） | 未纳入 |
| `PUT /monitor/job` | 有（`JobEdit`） | 有（`edit`） | `monitor:job:edit`（一致） | CRUD |
| `PUT /monitor/job/changeStatus` | 有（`JobChangeStatus`） | 有（`changeStatus`） | `monitor:job:changeStatus`（一致） | 未纳入 |
| `PUT /monitor/job/run` | 有（`JobRun`） | 有（`run`） | `monitor:job:changeStatus`（一致） | 未纳入 |
| `PUT /system/config` | 有（`ConfigEdit`） | 有（`edit`） | `system:config:edit`（一致） | CRUD |
| `PUT /system/dept` | 有（`DeptEdit`） | 有（`edit`） | `system:dept:edit`（一致） | CRUD |
| `PUT /system/dept/updateSort` | 有（`DeptUpdateSort`） | 有（`updateSort`） | `system:dept:edit`（一致） | 未纳入 |
| `PUT /system/dict/data` | 有（`DictDataEdit`） | 有（`edit`） | `system:dict:edit`（一致） | CRUD |
| `PUT /system/dict/type` | 有（`DictTypeEdit`） | 有（`edit`） | `system:dict:edit`（一致） | CRUD |
| `PUT /system/menu` | 有（`MenuEdit`） | 有（`edit`） | `system:menu:edit`（一致） | CRUD |
| `PUT /system/menu/updateSort` | 有（`MenuUpdateSort`） | 有（`updateSort`） | `system:menu:edit`（一致） | 未纳入 |
| `PUT /system/notice` | 有（`NoticeEdit`） | 有（`edit`） | `system:notice:edit`（一致） | CRUD |
| `PUT /system/post` | 有（`PostEdit`） | 有（`edit`） | `system:post:edit`（一致） | CRUD |
| `PUT /system/role` | 有（`RoleEdit`） | 有（`edit`） | `system:role:edit`（一致） | CRUD |
| `PUT /system/role/authUser/cancel` | 有（`RoleAuthUserCancel`） | 有（`cancelAuthUser`） | `system:role:edit`（一致） | 未纳入 |
| `PUT /system/role/authUser/cancelAll` | 有（`RoleAuthUserCancelAll`） | 有（`cancelAuthUserAll`） | `system:role:edit`（一致） | 未纳入 |
| `PUT /system/role/authUser/selectAll` | 有（`RoleAuthUserSelectAll`） | 有（`selectAuthUserAll`） | `system:role:edit`（一致） | 未纳入 |
| `PUT /system/role/changeStatus` | 有（`RoleChangeStatus`） | 有（`changeStatus`） | `system:role:edit`（一致） | 未纳入 |
| `PUT /system/role/dataScope` | 有（`RoleDataScope`） | 有（`dataScope`） | `system:role:edit`（一致） | 未纳入 |
| `PUT /system/user` | 有（`UserEdit`） | 有（`edit`） | `system:user:edit`（一致） | CRUD |
| `PUT /system/user/authRole` | 有（`UserAuthRoleSave`） | 有（`insertAuthRole`） | `system:user:edit`（一致） | 未纳入 |
| `PUT /system/user/changeStatus` | 有（`UserChangeStatus`） | 有（`changeStatus`） | `system:user:edit`（一致） | 未纳入 |
| `PUT /system/user/profile` | 有（`ProfileUpdate`） | 有（`updateProfile`） | 无（一致） | 未纳入 |
| `PUT /system/user/profile/updatePwd` | 有（`ProfileUpdatePwd`） | 有（`updatePwd`） | 无（一致） | 未纳入 |
| `PUT /system/user/resetPwd` | 有（`UserResetPwd`） | 有（`resetPwd`） | `system:user:resetPwd`（一致） | 未纳入 |

## 排除项

- `DELETE /test/user/{userId}（Swagger 内存演示接口）`
- `DELETE /tool/gen/{tableIds}（代码生成器）`
- `GET /test/user/list（Swagger 内存演示接口）`
- `GET /test/user/{userId}（Swagger 内存演示接口）`
- `GET /tool/gen/batchGenCode（代码生成器）`
- `GET /tool/gen/column/{tableId}（代码生成器）`
- `GET /tool/gen/db/list（代码生成器）`
- `GET /tool/gen/download/{tableName}（代码生成器）`
- `GET /tool/gen/genCode/{tableName}（代码生成器）`
- `GET /tool/gen/list（代码生成器）`
- `GET /tool/gen/preview/{tableId}（代码生成器）`
- `GET /tool/gen/synchDb/{tableName}（代码生成器）`
- `GET /tool/gen/{tableId}（代码生成器）`
- `POST /test/user/save（Swagger 内存演示接口）`
- `POST /tool/gen/createTable（代码生成器）`
- `POST /tool/gen/importTable（代码生成器）`
- `PUT /test/user/update（Swagger 内存演示接口）`
- `PUT /tool/gen（代码生成器）`
