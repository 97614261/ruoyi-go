-- v7 -> v8: permission data only; no schema changes or automatic grants.
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES
(6070,'导入草稿',6020,10,'#','',1,0,'F','0','0','cms:article:import','#','portal-init',NOW()),
(6071,'导出草稿',6020,11,'#','',1,0,'F','0','0','cms:article:export','#','portal-init',NOW()),
(6072,'导入草稿',6030,10,'#','',1,0,'F','0','0','cms:page:import','#','portal-init',NOW()),
(6073,'导出草稿',6030,11,'#','',1,0,'F','0','0','cms:page:export','#','portal-init',NOW()),
(6074,'导入草稿',6040,10,'#','',1,0,'F','0','0','cms:product:import','#','portal-init',NOW()),
(6075,'导出草稿',6040,11,'#','',1,0,'F','0','0','cms:product:export','#','portal-init',NOW());
