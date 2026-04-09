/*
<?php die(); ?>/*
MySQL Database Backup Tools
Server:127.0.0.1:3306
Database:www.19x.mm
Data:2022-01-26 13:48:01
*/

SET FOREIGN_KEY_CHECKS=0;
-- ----------------------------
-- Table structure for jz_article
-- ----------------------------
DROP TABLE IF EXISTS `jz_article`;
CREATE TABLE `jz_article` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`title` varchar(255) DEFAULT NULL COMMENT '文章标题',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '所属栏目',
`molds` varchar(50) DEFAULT 'article' COMMENT '模型标识',
`htmlurl` varchar(50) DEFAULT NULL COMMENT '栏目链接',
`keywords` varchar(255) DEFAULT NULL COMMENT '关键词',
`description` text COMMENT '简介',
`seo_title` varchar(255) DEFAULT NULL COMMENT 'SEO标题',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '管理员ID：0前台发布',
`litpic` varchar(255) DEFAULT NULL COMMENT '缩略图',
`body` mediumtext COMMENT '文章内容',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
`orders` int(4) NOT NULL DEFAULT '0' COMMENT '排序',
`hits` int(11) NOT NULL DEFAULT '0' COMMENT '点击次数',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否审核：1审核0未审2退回',
`comment_num` int(11) NOT NULL DEFAULT '0' COMMENT '评论数',
`istop` varchar(2) NOT NULL DEFAULT '0' COMMENT '是否置顶：1是0否',
`ishot` varchar(2) NOT NULL DEFAULT '0' COMMENT '是否头条：1是0否',
`istuijian` varchar(2) NOT NULL DEFAULT '0' COMMENT '是否推荐：1是0否',
`tags` varchar(255) DEFAULT NULL COMMENT 'TAG标签',
`member_id` int(11) NOT NULL DEFAULT '0' COMMENT '发布会员：0后台发布',
`target` varchar(255) DEFAULT NULL COMMENT '外链',
`ownurl` varchar(255) DEFAULT NULL COMMENT '自定义链接',
`jzattr` varchar(50) DEFAULT NULL COMMENT '推荐属性：1置顶2热点3推荐',
`tids` varchar(255) DEFAULT NULL COMMENT '副栏目',
`zan` int(11) DEFAULT '0' COMMENT '点赞数',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='文章表';
-- ----------------------------
-- Table structure for jz_attr
-- ----------------------------
DROP TABLE IF EXISTS `jz_attr`;
CREATE TABLE `jz_attr` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'attr' COMMENT '模型标识',
`name` varchar(50) DEFAULT NULL COMMENT '属性名',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='推荐属性';
-- ----------------------------
-- Table structure for jz_buylog
-- ----------------------------
DROP TABLE IF EXISTS `jz_buylog`;
CREATE TABLE `jz_buylog` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`aid` int(11) DEFAULT '0' COMMENT '内容ID',
`userid` int(11) DEFAULT '0' COMMENT '会员ID',
`orderno` varchar(255) DEFAULT NULL COMMENT '订单号',
`type` tinyint(1) DEFAULT '1' COMMENT '交易类型：1购买商品0兑换金币',
`buytype` varchar(20) DEFAULT NULL COMMENT '支付类型',
`msg` varchar(255) DEFAULT NULL COMMENT '记录',
`molds` varchar(255) DEFAULT NULL COMMENT '模型标识',
`amount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '总计',
`money` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '金额',
`addtime` int(11) DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='购买记录表';
-- ----------------------------
-- Table structure for jz_cachedata
-- ----------------------------
DROP TABLE IF EXISTS `jz_cachedata`;
CREATE TABLE `jz_cachedata` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`title` varchar(255) DEFAULT NULL COMMENT '标题',
`field` varchar(50) DEFAULT NULL COMMENT '字段',
`molds` varchar(50) DEFAULT NULL COMMENT '模型标识',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`isall` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否输出所有：1是0否',
`sqls` varchar(500) DEFAULT NULL COMMENT 'SQL',
`orders` varchar(255) DEFAULT NULL COMMENT '排序',
`limits` int(11) NOT NULL DEFAULT '10' COMMENT '输出条数',
`times` int(11) NOT NULL DEFAULT '0' COMMENT '更新周期',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='数据缓存表';
-- ----------------------------
-- Table structure for jz_chain
-- ----------------------------
DROP TABLE IF EXISTS `jz_chain`;
CREATE TABLE `jz_chain` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`title` varchar(100) DEFAULT NULL COMMENT '内链词',
`newtitle` varchar(100) DEFAULT NULL COMMENT '替换词',
`url` varchar(255) DEFAULT NULL COMMENT '链接',
`num` int(11) NOT NULL DEFAULT '-1' COMMENT '替换次数',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='内链';
-- ----------------------------
-- Table structure for jz_classtype
-- ----------------------------
DROP TABLE IF EXISTS `jz_classtype`;
CREATE TABLE `jz_classtype` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`classname` varchar(50) DEFAULT NULL COMMENT '栏目名',
`seo_classname` varchar(50) DEFAULT NULL COMMENT 'SEO栏目名',
`molds` varchar(50) DEFAULT NULL COMMENT '模型标识',
`litpic` varchar(255) DEFAULT NULL COMMENT '缩略图',
`description` text COMMENT '描述',
`keywords` varchar(255) DEFAULT NULL COMMENT '关键词',
`body` text COMMENT '内容',
`orders` int(4) NOT NULL DEFAULT '0' COMMENT '排序',
`orderstype` int(4) NOT NULL DEFAULT '0' COMMENT '排序类型：1时间倒序2ID正序3点击量倒序4ID正序5时间正序6点击量正序',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示',
`iscover` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否覆盖下级',
`pid` int(11) NOT NULL DEFAULT '0' COMMENT '上级栏目ID',
`gid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目权限：0不限制',
`htmlurl` varchar(50) DEFAULT NULL COMMENT '栏目链接',
`lists_html` varchar(50) DEFAULT NULL COMMENT '栏目页模板',
`details_html` varchar(50) DEFAULT NULL COMMENT '详情页模板',
`lists_num` int(4) DEFAULT '0' COMMENT '列表数量',
`comment_num` int(11) NOT NULL DEFAULT '0' COMMENT '评论数',
`gourl` varchar(255) DEFAULT NULL COMMENT '栏目外链',
`ishome` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否允许会员发布',
`isclose` tinyint(1) NOT NULL DEFAULT '0' COMMENT '关闭栏目',
`gids` varchar(255) DEFAULT NULL COMMENT '允许访问角色',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='栏目表';
-- ----------------------------
-- Table structure for jz_collect
-- ----------------------------
DROP TABLE IF EXISTS `jz_collect`;
CREATE TABLE `jz_collect` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`title` varchar(255) DEFAULT NULL COMMENT '标题',
`description` varchar(500) DEFAULT NULL COMMENT '简介',
`tid` int(11) DEFAULT NULL COMMENT '所属栏目',
`litpic` varchar(255) DEFAULT NULL COMMENT '缩略图',
`w` varchar(10) NOT NULL DEFAULT '0' COMMENT '宽',
`h` varchar(10) NOT NULL DEFAULT '0' COMMENT '高',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示：1显示0隐藏',
`url` varchar(255) DEFAULT NULL COMMENT '链接',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='轮播图';
-- ----------------------------
-- Table structure for jz_collect_type
-- ----------------------------
DROP TABLE IF EXISTS `jz_collect_type`;
CREATE TABLE `jz_collect_type` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`name` varchar(50) DEFAULT NULL COMMENT '分类名',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='轮播图分类';
-- ----------------------------
-- Table structure for jz_comment
-- ----------------------------
DROP TABLE IF EXISTS `jz_comment`;
CREATE TABLE `jz_comment` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'comment' COMMENT '模型标识',
`tid` int(4) NOT NULL DEFAULT '0' COMMENT '栏目tid',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '文章id',
`pid` int(11) NOT NULL DEFAULT '0' COMMENT '回复帖子id',
`zid` int(11) NOT NULL DEFAULT '0' COMMENT '主回复帖子：同一层楼内回复，规定主回复id',
`body` text COMMENT '评论内容',
`reply` text COMMENT '回复内容',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '发布时间',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '发布会员：0表示游客',
`likes` int(11) NOT NULL DEFAULT '0' COMMENT '点赞数',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示：1显示0隐藏2被删除',
`isread` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否已读：1已读0未读',
PRIMARY KEY (`id`),
KEY `tid` (`tid`),
KEY `aid` (`aid`),
KEY `pid` (`pid`),
KEY `zid` (`zid`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='评论表';
-- ----------------------------
-- Table structure for jz_ctype
-- ----------------------------
DROP TABLE IF EXISTS `jz_ctype`;
CREATE TABLE `jz_ctype` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`title` varchar(50) DEFAULT NULL COMMENT '配置栏名称',
`action` varchar(255) DEFAULT NULL COMMENT '配置标识，用于权限控制',
`sys` tinyint(1) DEFAULT 0 COMMENT '系统配置，1是0否',
`isopen` tinyint(1) DEFAULT 1 COMMENT '是否启用，1启用0关闭',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='系统设置栏目名';
-- ----------------------------
-- Table structure for jz_customurl
-- ----------------------------
DROP TABLE IF EXISTS `jz_customurl`;
CREATE TABLE `jz_customurl` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT NULL COMMENT '模型标识',
`url` varchar(255) DEFAULT NULL COMMENT '自定义URL',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '内容ID',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='自定义链接表';
-- ----------------------------
-- Table structure for jz_fields
-- ----------------------------
DROP TABLE IF EXISTS `jz_fields`;
CREATE TABLE `jz_fields` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`field` varchar(50) DEFAULT NULL COMMENT '字段标识',
`molds` varchar(50) DEFAULT NULL COMMENT '模型标识',
`fieldname` varchar(100) DEFAULT NULL COMMENT '字段名称',
`tips` varchar(100) DEFAULT NULL COMMENT '填写提示',
`fieldtype` tinyint(2) NOT NULL DEFAULT '1' COMMENT '输入类型',
`tids` text COMMENT '绑定栏目',
`fieldlong` varchar(50) DEFAULT NULL COMMENT '字段长度',
`body` text COMMENT '字段配置',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '表单排序',
`ismust` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否必填：1是0否',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '前台是否显示：1显示0隐藏',
`isadmin` tinyint(1) NOT NULL DEFAULT '1' COMMENT '后台是否显示：1显示0隐藏',
`issearch` tinyint(1) NOT NULL DEFAULT '0' COMMENT '搜索显示：1显示0隐藏',
`islist` tinyint(1) NOT NULL DEFAULT '0' COMMENT '列表显示：1显示0隐藏',
`format` varchar(50) DEFAULT NULL COMMENT '格式化',
`vdata` varchar(50) DEFAULT NULL COMMENT '默认值',
`isajax` tinyint(1) NOT NULL DEFAULT '1' COMMENT 'AJAX显示：1显示0隐藏',
`listorders` int(4) NOT NULL DEFAULT '0' COMMENT '列表排序',
`isext` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否扩展信息',
`width` varchar(50) DEFAULT NULL COMMENT '列表中显示宽度',
`ishome` tinyint(1) NOT NULL DEFAULT '1' COMMENT '前台表单录入',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4;
-- ----------------------------
-- Table structure for jz_hook
-- ----------------------------
DROP TABLE IF EXISTS `jz_hook`;
CREATE TABLE `jz_hook` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`module` varchar(50) DEFAULT NULL COMMENT '模块，Home/A',
`namespace` varchar(100) DEFAULT NULL COMMENT '控制器命名空间',
`controller` varchar(50) DEFAULT NULL COMMENT '控制器',
`action` varchar(255) DEFAULT NULL COMMENT '执行函数：可同时注册多个方法，逗号拼接',
`hook_namespace` varchar(100) DEFAULT NULL COMMENT '钩子控制器所在的命名空间',
`hook_controller` varchar(50) DEFAULT NULL COMMENT '钩子控制器',
`hook_action` varchar(50) DEFAULT NULL COMMENT '钩子执行方法',
`all_action` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否全局控制器',
`orders` int(4) NOT NULL DEFAULT '0' COMMENT '排序：越大越靠前执行',
`isopen` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否关闭：1开启0关闭',
`plugins_name` varchar(50) DEFAULT NULL COMMENT '关联插件名',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='插件钩子';
-- ----------------------------
-- Table structure for jz_layout
-- ----------------------------
DROP TABLE IF EXISTS `jz_layout`;
CREATE TABLE `jz_layout` (
`id` int(4) NOT NULL AUTO_INCREMENT,
`name` varchar(200) DEFAULT NULL COMMENT '桌面名称',
`top_layout` text COMMENT '顶部菜单',
`left_layout` text COMMENT '左侧菜单',
`gid` int(11) DEFAULT NULL COMMENT '所属角色',
`ext` varchar(255) DEFAULT NULL COMMENT '备注',
`sys` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否系统配置：1是0否',
`isdefault` tinyint(1) NOT NULL DEFAULT '0' COMMENT '默认配置：1是0否',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='桌面设置';
-- ----------------------------
-- Table structure for jz_level
-- ----------------------------
DROP TABLE IF EXISTS `jz_level`;
CREATE TABLE `jz_level` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'level' COMMENT '模型标识',
`name` varchar(20) DEFAULT NULL COMMENT '管理员名称',
`pass` varchar(100) DEFAULT NULL COMMENT '密码',
`tel` varchar(20) DEFAULT NULL COMMENT '电话号码',
`gid` int(4) NOT NULL DEFAULT '2' COMMENT '所属角色',
`email` varchar(50) DEFAULT NULL COMMENT '邮箱',
`regtime` int(11) NOT NULL DEFAULT '0' COMMENT '注册时间',
`logintime` int(11) NOT NULL DEFAULT '0' COMMENT '登录时间',
`status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态：1正常0冻结',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='管理员表';
-- ----------------------------
-- Table structure for jz_level_group
-- ----------------------------
DROP TABLE IF EXISTS `jz_level_group`;
CREATE TABLE `jz_level_group` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'level_group' COMMENT '模型标识',
`name` varchar(50) DEFAULT NULL COMMENT '角色名称',
`isadmin` tinyint(1) NOT NULL DEFAULT '0' COMMENT '超管：1是0否',
`ischeck` tinyint(1) NOT NULL DEFAULT '0' COMMENT '发布审核：1需要审核0不需要',
`classcontrol` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否配置栏目权限：1是0否',
`paction` text COMMENT '权限列表',
`tids` text COMMENT '拥有栏目权限',
`isagree` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态：1正常0冻结',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示',
`description` varchar(500) DEFAULT NULL COMMENT '描述',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='角色表';
-- ----------------------------
-- Table structure for jz_likes
-- ----------------------------
DROP TABLE IF EXISTS `jz_likes`;
CREATE TABLE `jz_likes` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '内容ID',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '会员ID',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`),
KEY `tid` (`tid`,`aid`,`userid`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='点赞表';
-- ----------------------------
-- Table structure for jz_link_type
-- ----------------------------
DROP TABLE IF EXISTS `jz_link_type`;
CREATE TABLE `jz_link_type` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`name` varchar(50) DEFAULT NULL COMMENT '友链分类名',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='友情链接分类表';
-- ----------------------------
-- Table structure for jz_links
-- ----------------------------
DROP TABLE IF EXISTS `jz_links`;
CREATE TABLE `jz_links` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`title` varchar(255) DEFAULT NULL COMMENT '友链名称',
`molds` varchar(50) DEFAULT 'links' COMMENT '模型标识',
`url` varchar(255) DEFAULT NULL COMMENT '链接',
`isshow` tinyint(1) DEFAULT '1' COMMENT '是否显示：1显示0隐藏',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '管理员ID',
`htmlurl` varchar(50) DEFAULT NULL COMMENT '栏目链接',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
`member_id` int(11) NOT NULL DEFAULT '0' COMMENT '会员ID',
`target` varchar(255) DEFAULT NULL COMMENT '外链',
`ownurl` varchar(255) DEFAULT NULL COMMENT '自定义链接',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '发布时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='友情链接表';
-- ----------------------------
-- Table structure for jz_member
-- ----------------------------
DROP TABLE IF EXISTS `jz_member`;
CREATE TABLE `jz_member` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'member' COMMENT '模型标识',
`username` varchar(50) DEFAULT NULL COMMENT '用户昵称',
`openid` varchar(255) DEFAULT NULL COMMENT '微信OPENID',
`pass` varchar(255) DEFAULT NULL COMMENT '密码',
`token` varchar(255) DEFAULT NULL COMMENT 'Token',
`sex` tinyint(1) NOT NULL DEFAULT '0' COMMENT '性别：1男2女0未知',
`gid` int(11) NOT NULL DEFAULT '1' COMMENT '会员分组ID',
`litpic` varchar(255) DEFAULT NULL COMMENT '头像',
`tel` varchar(50) DEFAULT NULL COMMENT '手机号码',
`jifen` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '积分数',
`likes` text COMMENT '喜欢点赞（已废弃）',
`collection` text COMMENT '收藏（已废弃）',
`money` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '金币',
`email` varchar(255) DEFAULT NULL COMMENT '邮箱',
`address` varchar(255) DEFAULT NULL COMMENT '地址',
`province` varchar(50) DEFAULT NULL COMMENT '省份',
`city` varchar(50) DEFAULT NULL COMMENT '城市',
`regtime` int(11) NOT NULL DEFAULT '0' COMMENT '注册时间',
`hassendtime` int(11) NOT NULL DEFAULT '0' COMMENT '发送验证码时间',
`logintime` int(11) NOT NULL DEFAULT '0' COMMENT '登录时间',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态：1正常0封禁',
`signature` varchar(255) DEFAULT NULL COMMENT '个性签名',
`birthday` varchar(25) DEFAULT NULL COMMENT '生日：2020-01-01',
`follow` text COMMENT '关注列表',
`fans` int(11) NOT NULL DEFAULT '0' COMMENT '粉丝数',
`ismsg` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启接收消息提醒',
`iscomment` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启接收评论消息提醒',
`iscollect` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启接收收藏消息提醒',
`islikes` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启接收点赞消息提醒',
`isat` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启接收@消息提醒',
`isrechange` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启接收交易消息提醒',
`pid` int(11) NOT NULL DEFAULT '0' COMMENT '推荐用户ID',
`uploadsize` int(11) NOT NULL DEFAULT '50' COMMENT '上传大小限制',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='会员表';
-- ----------------------------
-- Table structure for jz_member_group
-- ----------------------------
DROP TABLE IF EXISTS `jz_member_group`;
CREATE TABLE `jz_member_group` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'member_group' COMMENT '模型标识',
`name` varchar(50) DEFAULT NULL COMMENT '分组名',
`description` varchar(255) DEFAULT NULL COMMENT '分组简介',
`paction` text COMMENT '权限',
`pid` int(11) NOT NULL DEFAULT '0' COMMENT '分组上级',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示',
`isagree` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否允许登录',
`iscomment` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否允许评论',
`ischeckmsg` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否需要审核评论',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
`discount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '折扣价：现金折扣或者百分比折扣',
`discount_type` tinyint(1) NOT NULL DEFAULT '0' COMMENT '折扣类型：0无折扣1现金折扣,1百分比折扣',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='会员分组';
-- ----------------------------
-- Table structure for jz_menu
-- ----------------------------
DROP TABLE IF EXISTS `jz_menu`;
CREATE TABLE `jz_menu` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`name` varchar(255) DEFAULT NULL COMMENT '导航名称',
`nav` text COMMENT '导航配置',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示：1显示0不显示',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='导航表';
-- ----------------------------
-- Table structure for jz_message
-- ----------------------------
DROP TABLE IF EXISTS `jz_message`;
CREATE TABLE `jz_message` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'message' COMMENT '模型标识',
`title` varchar(255) DEFAULT NULL COMMENT '标题',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '发布会员',
`tid` int(4) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '文章ID',
`user` varchar(255) DEFAULT NULL COMMENT '用户名',
`ip` varchar(255) DEFAULT NULL COMMENT 'IP',
`body` text COMMENT '留言内容',
`tel` varchar(50) DEFAULT NULL COMMENT '电话',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '发布时间',
`orders` int(4) NOT NULL DEFAULT '0' COMMENT '排序',
`email` varchar(255) DEFAULT NULL COMMENT '邮箱',
`isshow` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否审核：1审核0未审',
`istop` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否置顶：1是0否',
`hits` int(11) NOT NULL DEFAULT '0' COMMENT '点击量',
`tids` varchar(255) DEFAULT NULL COMMENT '副栏目',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='留言表';
-- ----------------------------
-- Table structure for jz_molds
-- ----------------------------
DROP TABLE IF EXISTS `jz_molds`;
CREATE TABLE `jz_molds` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`name` varchar(50) DEFAULT NULL COMMENT '模型名称',
`biaoshi` varchar(50) DEFAULT NULL COMMENT '模型标识',
`sys` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否系统：1是0否',
`isopen` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开启：1开启0关闭',
`iscontrol` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否开启权限：1开启权限0不开启',
`ismust` tinyint(1) NOT NULL DEFAULT '0' COMMENT '栏目必选：1是0否',
`isclasstype` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示栏目',
`isshowclass` tinyint(1) DEFAULT '1' COMMENT '栏目绑定：1显示0隐藏',
`list_html` varchar(50) DEFAULT 'list.html' COMMENT '默认列表模板',
`details_html` varchar(50) DEFAULT 'details.html' COMMENT '默认详情模板',
`orders` int(11) NOT NULL DEFAULT '100' COMMENT '排序',
`ispreview` tinyint(1) DEFAULT '1' COMMENT '是否可以预览',
`ishome` tinyint(1) DEFAULT '0' COMMENT '前台发布',
PRIMARY KEY (`id`),
KEY `biaoshi` (`biaoshi`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='模型表';
-- ----------------------------
-- Table structure for jz_orders
-- ----------------------------
DROP TABLE IF EXISTS `jz_orders`;
CREATE TABLE `jz_orders` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'orders' COMMENT '模型标识',
`orderno` varchar(255) DEFAULT NULL COMMENT '订单号',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '下单会员',
`paytype` varchar(20) DEFAULT NULL COMMENT '支付方式',
`ptype` tinyint(1) DEFAULT '1' COMMENT '交易类型：1商品购买2充值金额3充值积分',
`tel` varchar(50) DEFAULT NULL COMMENT '电话',
`username` varchar(50) DEFAULT NULL COMMENT '用户名',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`price` varchar(200) DEFAULT NULL COMMENT '价格',
`jifen` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '积分',
`qianbao` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '钱包',
`body` text COMMENT '购买内容',
`receive_username` varchar(50) DEFAULT NULL COMMENT '收件人',
`receive_tel` varchar(20) DEFAULT NULL COMMENT '收件电话',
`receive_email` varchar(50) DEFAULT NULL COMMENT '收件邮箱',
`receive_address` varchar(255) DEFAULT NULL COMMENT '收件地址',
`ispay` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否支付：1支付0未支付',
`paytime` int(11) NOT NULL DEFAULT '0' COMMENT '支付时间',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '下单时间',
`send_time` int(11) NOT NULL DEFAULT '0' COMMENT '发货时间',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '订单状态：1提交订单,2已支付,3超时,4已提交订单,5已发货,6已废弃失效,0删除订单',
`discount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '折扣',
`yunfei` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '运费',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='订单表';
-- ----------------------------
-- Table structure for jz_page
-- ----------------------------
DROP TABLE IF EXISTS `jz_page`;
CREATE TABLE `jz_page` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'page' COMMENT '模型标识',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`htmlurl` varchar(50) DEFAULT NULL COMMENT '链接',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
`member_id` int(11) NOT NULL DEFAULT '0' COMMENT '用户ID',
`isshow` tinyint(1) DEFAULT '1' COMMENT '是否显示：1显示0隐藏',
`istop` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否置顶：1是0否',
`hits` int(11) NOT NULL DEFAULT '0' COMMENT '点击量',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '发布时间',
`tids` varchar(255) NOT NULL COMMENT '副栏目',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='单页模型';
-- ----------------------------
-- Table structure for jz_pictures
-- ----------------------------
DROP TABLE IF EXISTS `jz_pictures`;
CREATE TABLE `jz_pictures` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '内容ID',
`molds` varchar(50) DEFAULT NULL COMMENT '模型标识',
`path` varchar(20) DEFAULT 'Admin' COMMENT '板块：Admin后台Home前台',
`filetype` varchar(20) DEFAULT NULL COMMENT '类型',
`size` varchar(50) DEFAULT NULL COMMENT '大小',
`litpic` varchar(255) DEFAULT NULL COMMENT '链接',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '管理员ID/发布会员ID',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='图片集';
-- ----------------------------
-- Table structure for jz_pingjia
-- ----------------------------
DROP TABLE IF EXISTS `jz_pingjia`;
CREATE TABLE `jz_pingjia` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`tid` int(11) DEFAULT '0' COMMENT '所属栏目',
`tids` varchar(255) DEFAULT NULL COMMENT '副栏目',
`title` varchar(255) DEFAULT NULL COMMENT '标题',
`litpic` varchar(255) DEFAULT NULL COMMENT '缩略图',
`keywords` varchar(255) DEFAULT NULL COMMENT '关键词',
`description` varchar(500) DEFAULT NULL COMMENT '简介',
`body` text COMMENT '内容',
`molds` varchar(50) DEFAULT 'pingjia' COMMENT '模型标识',
`userid` int(11) DEFAULT '0' COMMENT '发布管理员',
`orders` int(11) DEFAULT '0' COMMENT '排序',
`member_id` int(11) DEFAULT '0' COMMENT '前台用户',
`comment_num` int(11) DEFAULT '0' COMMENT '评论数',
`htmlurl` varchar(100) DEFAULT NULL COMMENT '栏目链接',
`isshow` tinyint(1) DEFAULT '1' COMMENT '是否显示',
`target` varchar(255) DEFAULT NULL COMMENT '外链',
`ownurl` varchar(255) DEFAULT NULL COMMENT '自定义URL',
`jzattr` varchar(50) DEFAULT NULL COMMENT '推荐属性',
`hits` int(11) DEFAULT '0' COMMENT '点击量',
`zan` int(11) DEFAULT '0' COMMENT '点赞数',
`tags` varchar(255) DEFAULT NULL COMMENT 'TAG',
`addtime` int(11) DEFAULT '0' COMMENT '发布时间',
`zhiye` varchar(255) DEFAULT NULL,
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4;
-- ----------------------------
-- Table structure for jz_plugins
-- ----------------------------
DROP TABLE IF EXISTS `jz_plugins`;
CREATE TABLE `jz_plugins` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`name` varchar(50) DEFAULT NULL COMMENT '插件名称',
`filepath` varchar(50) DEFAULT NULL COMMENT '插件文件名',
`description` varchar(255) DEFAULT NULL COMMENT '简介',
`version` decimal(3,1) NOT NULL DEFAULT '0.0' COMMENT '版本',
`author` varchar(50) DEFAULT NULL COMMENT '作者',
`update_time` int(11) NOT NULL DEFAULT '0' COMMENT '更新时间',
`module` varchar(20) NOT NULL DEFAULT 'Home' COMMENT '模块',
`isopen` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否开启：1开启0关闭',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '发布时间',
`config` text COMMENT '配置',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='插件表';
-- ----------------------------
-- Table structure for jz_power
-- ----------------------------
DROP TABLE IF EXISTS `jz_power`;
CREATE TABLE `jz_power` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`action` varchar(50) DEFAULT NULL COMMENT '函数名',
`name` varchar(50) DEFAULT NULL COMMENT '权限名',
`pid` int(11) NOT NULL DEFAULT '0' COMMENT '父类权限ID',
`isagree` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否开放',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='用户权限表';
-- ----------------------------
-- Table structure for jz_product
-- ----------------------------
DROP TABLE IF EXISTS `jz_product`;
CREATE TABLE `jz_product` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`molds` varchar(50) DEFAULT 'product' COMMENT '模型标识',
`title` varchar(255) DEFAULT NULL COMMENT '商品名称',
`seo_title` varchar(255) DEFAULT NULL COMMENT 'SEO标题',
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '所属栏目',
`hits` int(11) NOT NULL DEFAULT '0' COMMENT '点击量',
`htmlurl` varchar(50) DEFAULT NULL COMMENT '栏目链接',
`keywords` varchar(255) DEFAULT NULL COMMENT '关键词',
`description` varchar(255) DEFAULT NULL COMMENT '简介',
`litpic` varchar(255) DEFAULT NULL COMMENT '首图',
`stock_num` int(11) NOT NULL DEFAULT '0' COMMENT '库存',
`price` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '价格',
`pictures` text COMMENT '图集',
`isshow` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否显示：1显示0不显示',
`comment_num` int(11) NOT NULL DEFAULT '0' COMMENT '评论数',
`body` mediumtext COMMENT '详情',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '录入管理员ID',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '更新时间',
`istop` varchar(2) NOT NULL DEFAULT '0' COMMENT '是否置顶：1是0否',
`ishot` varchar(2) NOT NULL DEFAULT '0' COMMENT '是否头条：1是0否',
`istuijian` varchar(2) NOT NULL DEFAULT '0' COMMENT '是否推荐：1是0否',
`tags` varchar(255) DEFAULT NULL COMMENT 'TAG标签',
`member_id` int(11) NOT NULL DEFAULT '0' COMMENT '发布会员',
`target` varchar(255) DEFAULT NULL COMMENT '外链',
`ownurl` varchar(255) DEFAULT NULL COMMENT '自定义链接',
`jzattr` varchar(50) DEFAULT NULL COMMENT '推荐属性：1置顶2热点3推荐',
`tids` varchar(255) DEFAULT NULL,
`zan` int(11) DEFAULT '0',
`lx` varchar(2) DEFAULT NULL,
`color` varchar(2) DEFAULT NULL,
`hy` varchar(500) DEFAULT NULL,
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='商品表';
-- ----------------------------
-- Table structure for jz_recycle
-- ----------------------------
DROP TABLE IF EXISTS `jz_recycle`;
CREATE TABLE `jz_recycle` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`title` varchar(255) DEFAULT NULL COMMENT '标记',
`molds` varchar(50) DEFAULT NULL COMMENT '回收模型标志',
`data` mediumtext COMMENT '回收数据',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '删除时间',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '关联删除',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='回收站';
-- ----------------------------
-- Table structure for jz_ruler
-- ----------------------------
DROP TABLE IF EXISTS `jz_ruler`;
CREATE TABLE `jz_ruler` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`name` varchar(50) DEFAULT NULL COMMENT '权限名称',
`fc` varchar(50) DEFAULT NULL COMMENT '函数',
`pid` int(11) NOT NULL DEFAULT '0' COMMENT '父类权限',
`isdesktop` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否桌面配置显示（已废弃）',
`sys` tinyint(1) NOT NULL DEFAULT '0' COMMENT '系统：1是0否',
PRIMARY KEY (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8mb4 COMMENT='角色权限表';
-- ----------------------------
-- Table structure for jz_shouchang
-- ----------------------------
DROP TABLE IF EXISTS `jz_shouchang`;
CREATE TABLE `jz_shouchang` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`tid` int(11) NOT NULL DEFAULT '0' COMMENT '栏目ID',
`aid` int(11) NOT NULL DEFAULT '0' COMMENT '内容ID',
`userid` int(11) NOT NULL DEFAULT '0' COMMENT '会员ID',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='用户收藏表';
-- ----------------------------
-- Table structure for jz_sysconfig
-- ----------------------------
DROP TABLE IF EXISTS `jz_sysconfig`;
CREATE TABLE `jz_sysconfig` (
`id` int(11) NOT NULL AUTO_INCREMENT,
`field` varchar(50) DEFAULT NULL COMMENT '配置字段',
`title` varchar(255) DEFAULT NULL COMMENT '配置名称',
`tip` varchar(255) DEFAULT NULL COMMENT '字段填写提示',
`type` tinyint(1) NOT NULL DEFAULT '0' COMMENT '参数类型：1图片2单行文本3多行文本4编辑器5文件上传6下拉开启关闭选项7下拉是否选项8栏目选项9代码',
`data` text COMMENT '配置内容',
`typeid` tinyint(1) NOT NULL DEFAULT '0' COMMENT '配置栏ID',
`config` text COMMENT '单选多选配置信息',
`orders` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
`sys` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否系统字段',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='系统配置';
-- ----------------------------
-- Table structure for jz_tags
-- ----------------------------
DROP TABLE IF EXISTS `jz_tags`;
CREATE TABLE `jz_tags` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`tid` int(11) DEFAULT '0' COMMENT '栏目ID',
`tids` varchar(500) DEFAULT NULL COMMENT '相关栏目',
`orders` int(11) DEFAULT '0' COMMENT '排序',
`comment_num` int(11) DEFAULT '0' COMMENT '评论数',
`molds` varchar(50) DEFAULT 'tags' COMMENT '模型标识',
`htmlurl` varchar(100) DEFAULT NULL COMMENT '栏目链接',
`keywords` varchar(50) DEFAULT NULL COMMENT '关键词',
`newname` varchar(50) DEFAULT NULL COMMENT '替换词（已废弃）',
`num` int(4) DEFAULT '-1' COMMENT '替换次数：-1不限制',
`isshow` tinyint(1) DEFAULT '1' COMMENT '是否显示：1显示隐藏',
`target` varchar(50) DEFAULT '_blank' COMMENT '外链',
`number` int(11) DEFAULT '0' COMMENT '数量',
`member_id` int(11) DEFAULT '0' COMMENT '发布会员',
`ownurl` varchar(255) DEFAULT NULL COMMENT '自定义链接',
`tags` varchar(255) DEFAULT NULL COMMENT 'TAG标签',
`addtime` int(11) NOT NULL DEFAULT '0' COMMENT '添加时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='TAGS表';
-- ----------------------------
-- Table structure for jz_task
-- ----------------------------
DROP TABLE IF EXISTS `jz_task`;
CREATE TABLE `jz_task` (
`id` int(11) unsigned NOT NULL AUTO_INCREMENT,
`tid` int(11) DEFAULT '0' COMMENT '栏目ID',
`aid` int(11) DEFAULT '0' COMMENT '文章ID',
`userid` int(11) DEFAULT '0' COMMENT '发布会员',
`puserid` int(11) DEFAULT '0' COMMENT '对象会员',
`molds` varchar(50) DEFAULT NULL COMMENT '模块标识',
`type` varchar(50) DEFAULT NULL COMMENT '消息类型',
`body` varchar(255) DEFAULT NULL COMMENT '内容',
`url` varchar(255) DEFAULT NULL COMMENT '链接',
`isread` tinyint(1) DEFAULT '0' COMMENT '是否已读：1已读0未读',
`isshow` tinyint(1) DEFAULT '1' COMMENT '是否显示：1显示0隐藏',
`readtime` int(11) DEFAULT '0' COMMENT '阅读时间',
`addtime` int(11) DEFAULT '0' COMMENT '发布时间',
PRIMARY KEY (`id`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8mb4 COMMENT='会员消息表';
-- ----------------------------
-- Records of jz_article
-- ----------------------------
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('1','如何有效的提高网站权重？','8', NULL,'znxw', NULL,'想要在搜索引擎的排名更靠前、提高整站的流量、提高用户对网站的信任度，所以提高网站的权重是相当重要。如何才能够快速的提高自己网站的权重，或者说哪些东西是决定网站权重的重要因素。','如何有效的提高网站权重？','1','/static/upload/2022/01/20/202201202461.jpg','
<p>如何有效的提高网站的权重？想要在搜索引擎的排名更靠前、提高整站的流量、提高用户对网站的信任度，所以提高网站的权重是相当重要。如何才能够快速的提高自己网站的权重，或者说哪些东西是决定网站权重的重要因素，网站合理的内链结构布局。</p>
<p style="text-align: center;"><img src="/static/upload/2022/01/20/202201202799.png"/></p><p>
    目前来讲，外链的地位已不再像之前那样是SEO优化的核心，外链为皇的时代早就过去了，如今更为重要的就是网站内容，而内链就好比一张蜘蛛网一样，起着连接和传递网站系统化内容的作用。所以，内链设置必须注重合理、呼应，避免重复、堆积，这样更利于搜索引擎的抓取和收录。</p>
<p><br/></p><p>
    从目前掌握的情况而言，很多人都喜欢给首页做一个超链接，做回链是需要掌握一个度的，一般情况下，一个独立页面做上1-2个内链就可以了，导航以及面包屑导航就是最好的内链，因此，在做内链的时候千万别滥用，否则会被百度惩罚，认为你的网站是为了做排名而做排名，不具备用户搜索的参考价值。优化建议：做好具有引导性的内链，大量的回链犹如“七伤拳”，用得好利于排名，用不好则属于自残，在这里，建议广大的SEO千万不要在底部或者文章中做太多的内链，要做符合用户搜索的内容。</p>
<p><br/></p><p>好的域名及稳定的服务器和打开速度。</p><p><br/></p><p>
    对于优化而言，好的域名主要是指域名中包含关键词或者企业名称，最好简短易记。其次，就是老域名和新域名的区分，当然老域名更利于优化。域名只是影响优化的一小部分，而网站服务器的稳定性和打开速度却是极为重要的一部分。数据调查显示，通常一个打开速度较慢的站点会减少60%的流量，而且网站一旦出现服务器异常，打不开，可能就会造成对搜索引擎的不友好现象，收录成为困难。</p>
<p><br/></p><p>有规律的产生日常更新维护。</p><p><br/></p><p>
    如今的搜索引擎更注重高质量的原创内容，而高质量的标准取决于可读性、稀缺性、价值性三个方面。所以，大家在更新网站内容的时候要把握好这几点，高质量的原创内容一直是网站用户和搜索引擎喜欢的，当然这里说的原创也并非原创，很多朋友都能理解这块，比如某个网站发布一篇类似文章，但由于对方排版不清晰该插入图片或视频的地方未进行操作密密麻麻的文章给人的一种感觉都不是很友好，此时我们又需要这篇文章怎么办，解决对方未完成的细节问题再发布，搜索引擎依然会认为你的文章比你抄袭的文章更有价值意思。</p>
<p><br/></p><p>美观、有逻辑性的排版布局。</p><p><br/></p><p>
    因为现在的网民审美及网站功能各方面的要求提高，美观似乎已成为每个网站的基本要求，因为只有满足了用户的浏览及感官体验，才能达到所谓的用户体验和粘度。但是美观并不代表就一定有酷炫的功能和风格，因为JS、FLASH等特效方式的渲染力虽大于图片，但是搜索引擎是抓不到，对搜索引擎来说是不友好的。所以，在保证美观、逻辑性的排版布局的同时，JS等特殊效果尽量少用。但目前从泽民了解的情况而言JS百度也是可以抓取的。</p>
<p><br/></p><p>
    有些朋友每天更新文章，但排版乱七八糟，字体忽大忽小，要么就是文字过多，一张图片也没有，密密麻麻的文字，或者是文字小得可怜，正常情况下，14px-16px的字体是最适合用户阅读的，建议大家在为网站更新文章的时候，最好用图文并茂的模式，排版干净整洁，赢得客户的第一印象，搜索引擎也会根据页面的整洁度给予好的评分。</p>
<p><br/></p><p>合理利用优化标签。</p><p><br/></p><p>
    你是否合理的运用了这些标签?很多人不会用，也有很多人会用但使用过度了，标签是优化常用的一个标签，在单页面优化中，它的存在也是对页面优化起到了很大的促进作用，在最能突出页面内容的地方加上
    会让搜索引擎优先抓取，然后在一层一层往下面抓取，会让搜索引擎更好的了解该页面的核心内容，但一个页面只能有一对 ，至于 可以使用多次，但要使用合理。</p><p><br/></p><p>三大标签TDK的正确写。</p><p><br/>
</p><p>
    我想这个时候肯定有人会问我为什么把标题写法最主要的一点写在最后，正因为重要我才写到最好，判断一个合格的SEO人员首先是看你写的标题是否完美，常见的标题写法就是把公司做的产品词全部写在标题上，这是百分之80SEO人员的通病，百度在之前对标题的写法做出的回应具体如下：</p>
<p><br/></p><p>网站首页title的写法：网站标题 ?或者 ?网站标题_服务词或者产品词;</p><p><br/></p><p>网站频道页title的写法：频道名称_网站名称;</p><p><br/></p><p>
    网站文章页title的写法：文章标题_频道名称_网站名称;</p><p><br/></p><p>这种写法符合重要的内容放在title前面，权重从左到右依次递减的规则。</p><p><br/></p><p>
    这里在补充一点，在写标题的时候一定要考虑到百度的分词算法，很多人都不知道，分词的规则：a，在百度搜索一个三个以三个以下汉字的关键词，百度不会对关键词进行分割，百度显示的是所有匹配完整关键词的搜索结果;b，在百度搜索四个汉字以上的关键词，百度会对关键词进行分割，百度会显示完整关键词和组合关键词的搜索结果;分词后组合的方式有非常多种，对我们SEO来说，最有价值的还是分词的正向最大匹配法以及逆向匹配法。说白一点，就是title在分词后，可以正向和反向的组合不同的关键词。</p>
<p><br/></p><p>
    优质的内外链接：我们的内链如果做得好话，让我们更加容易的找到自己想要的资料，让网民更好的阅读我们的文章，这样网民停留在我们网页的时间也变久了，对于权重的提升很有帮助优质是外链要从友情链接做起，不求多但求精，质量重于数量，多寻找一些高质量的友情链接，不仅能提升网站权重，还能辅助相关的关键字提升。</p>
<p><br/></p>','1642639144','0','10','1','0','0','0','0','SEO','0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('2','SEO优化细节问题','8', NULL,'znxw', NULL,'我们可以发现，现在很多企业站都在做SEO优化，让自己的网站能让更多的人搜索到，但是当我们在优化的过程中，有时会由于一些操作而带来一些影响，有些细节问题大家一定要注意，我们一起来了解一下。','SEO优化细节问题','1','/static/upload/2022/01/20/202201206807.jpg','
<p>1.H1、H2、H3标签的使用，这个对于一个seo人员来说也是需要了解的基本功能。它可以加重对关键词的描述，通俗一点说就是能够更好的集中网站的权重。</p><p><br/></p><p>
    2.META标签是HTML标记头部区的一个关键标签，提供文档字符集、使用语言、作者等基本信息，以及对关键词和网页等级的设定等，最大的作用是能够做搜索引擎优化(SEO)。</p><p><br/></p><p>
    3.title标签能够让搜索引擎在栏目上显示你提交的文字，也就是咱们经常说到的标题前缀。</p><p><br/></p><p>4.处理图片Alt、title标签以及为页面添加元标记meta
    网站经常都会更新图片，因此为图片增加alt是非常重要的，这样能够更好的让搜索引擎识别你网站上的图片。</p><p><br/></p><p>
    5.大家都知道人的搜索习惯是通过关键词、标题、描述来进行搜索，因此定位好你每个页面的标题、关键词、描述也是必不可少的一环。同时还需要注意的是个别关键词的密度问题。</p><p><br/></p><p>
    6.网站地图专业名字也叫做sitemap 这个可以通过一些专业的工具去生成，然后提交给搜索引擎，使得搜索引擎对你的网站更加友好。</p><p><br/></p><p>
    7.404页面设置、301重定向这两个主要在于提高网站体验度这方面，当你的网站出现死链，或者访问不了的情况那么就需要用到404跳转页面了。</p><p><br/></p><p>
    8.Robot.txt如果你没有接触过seo行业看起来一定很陌生，Robot.txt所起的作用是至关重要的，他能让搜索引擎更好的识别你的网站哪些内容是该抓取收录的，哪些内容是不能抓取收录的。</p><p><img
            src="/static/upload/image/20220120/1642640161966799.jpeg" title="1642640161966799.jpeg" alt="seo-1.jpeg"/>
</p><p>网站首页被K的原因</p><p><br/></p><p>
    在外链方面应当循序渐进.并且有规律的增长，如果突然之间失去了一些外链的话，就会造成网站降权或者被K，所以说要其有广泛性，不要在一个网站上拴住。当然做的外链在权重高的网站那是肯定的好了，但要注重质量，自己把握的住就好。</p><p>
    <br/></p><p>使用了一些作弊手段，这也会导致你的站点被降权或K站。如果你没有做这些，也许是竟争对手用这些手段在陷害你的站，这得注意下，经常查查外链情况，如果有垃圾外链过来的话，就用百度外链工具屏蔽掉吧。</p><p>
    <br/></p><p>
    服务器影响：如果使用的空间同IP中有大量博彩类网站，或者有网站被降权或被K，都有可能影响到同iP下你的网站，这个影响的程度我也说不好，这点可注意一下。服务器的不稳定因素也是排名提不上去或被降权的原因之一，服务器的不稳定，造成搜索无法正常访问，故而做出惩罚。</p>
<p><br/></p><p>关键词密度问题：并且要注意不要堆砌关键词，关键词的密度应当&lt;8%，这个可以使用工具查询的到，其实这个关键词的密度，大家就仁者见仁智者见智了。</p><p><br/></p><p>
    网站内容问题：多写意些高质量的相关原创文章，增加搜索引擎的友好度，伪原创和转载的那肯定是不行的，尤其是新网站就更不能进行文章采集。同时要注重网站内部链优化，还要就是不要在网站不稳定期间更换模板，不然蜘蛛又需要重新抓取，影响不好。如果是新站的话，文章当中还不要放太多的链接，尤其是链接到首页的。</p>
<p><br/></p>','1642640121','0','1','1','0','0','0','0','SEO','0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('3','如何正确选择关键词技巧？','8', NULL,'znxw', NULL,'一个要做SEO优化的网站，如果在之前不能够选择好适当的关键词的话，那么对网站的优化会造成很大的影响，因此就会一步走错，满盘皆输的窘境。结合网站本身从用户的角度出发选择优化词网站调整完毕以后主要的流量以及权...','如何正确选择关键词技巧？','1','/static/upload/2022/01/20/202201209692.jpg','
<p>
    一个要做SEO优化的网站，如果在之前不能够选择好适当的关键词的话，那么对网站的优化会造成很大的影响，因此就会一步走错，满盘皆输的窘境。结合网站本身从用户的角度出发选择优化词网站调整完毕以后主要的流量以及权重的来源便是用户，那么我们应该怎么去让用户精准的查询到网站呢?这就要我们去站到用户的角度去考虑网站产品本身需要拓展的关键词了，精准的抓住用户的需求是企业网站有价值体现的最要表现。</p>
<p><br/></p><p>合理的选择优化关键词</p><p><br/></p><p>
    合理选择优化关键词，主要针对于中小型企业网站来说，我们不能一味的去选择指数高、搜索量大的词，前期尽量去选择一些优化关键词的长尾关键词，通过不断的提升网站的基础从而提升选择关键词的优化难度。确定关键词方向，从百度下拉框中选择精准关键词我们在确定好客户网站关键词方针以后，将关键词输入到百度搜索框中，自动弹出下拉框，下拉框中的词就是我们日常用户搜索量比较大的词。</p>
<p><br/></p><p>根据客户网站内容进行关键词扩展</p><p><br/></p><p>
    我们在为客户的网站选择关键词的时候，要根据客户网站主要做的产品、服务内容来选定大的方针，在利用一些关键词扩展工具选择一些比较多的关键词，在从中筛选出有价值的关键词。企业在进行网站关键词优化的时候，应该注重一些网络优化的细节问题，不同的网站，其优化方式有很大的不同，对于一些优化细节的问题是不容忽视的。同时要做好网站优化数据的统计与分析，这将才能够更好的做好网站优化效果。</p>
<p><br/></p><p>企业在进行网络推广的时候，大部分企业网站的排名都不尽人意，同时网站优化的效果差，周期长，这是现今大多数企业的通病，长期如此的话，企业也就逐渐丧失了继续优化下去的信心。</p><p><br/></p><p>
    做网站推广的过程中，需要注重很多相关的细节，就好比网站锚文本建设，合理的运用网站锚文本，对搜索引擎进行引导，这样也就能够促进蜘蛛的更快更精准的抓取网站内容，从而提高长尾关键词的排名、增加网站的权重。但是做网站锚文本也不是随意增加的，网站页面的相关内容能够通过锚文本，精准地指向相关网站页面的内容，可以说是锚文本对网站页面所做的内容评价。</p>
<p><br/></p><p>锚文本在内外链的用处</p><p><br/></p><p>
    内部链接内部链接的作用是提高网站的网页和网页之间增加粘度，提高用户的体验度。文章包含其他文章的关键词，那都可以做锚文本链接。一定要注意锚文本链接的多样化，不要都是指向首页的锚文本链接，这样对你的网站也是没有好处的。(内部链接的规则)</p>
<p><br/></p><p>外部链接外链现如今越来越不好做，个人签名和网页的网址链接现在都被百度算作了垃圾外链。锚文本链接是权重最高的链接，但是一定不要优化过度了，否则会过犹不及。</p><p><br/></p><p>
    锚文本怎么做?</p><p><br/></p><p>
    锚文本运用的地方很广，我们要把它做到：网站导航、栏目、分类目录、次导航、外部链接、友情链接、文章内的锚文本。文章内锚文本频率：站内锚文本我们提倡是控制在1%频率就好;锚文本链接放置位置：我们一般在一篇文章第一次出现这个关键词时就做成锚文本;锚文本链接数量：一篇文章如果出现多次相同的关键词，我们只做一个就好。做网站锚文本建设其实难度并不大，关键是要掌握锚文本建设的方法，合理的运用，这样才能够个网站优化效果带来很大的助益。如果为了加快网站排名的提升，随意增加锚文本数量，同时链接的相关性也比较差，自然搜索引擎就会以为存在作弊行为，就会给予相应的惩罚。</p>
<p><br/></p>','1642640201','0','5','1','0','0','0','0','SEO','0', NULL, NULL, NULL,',9,','0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('4','友链交换方法及友链交换平台有哪些？','9', NULL,'xyxw', NULL,'一些SEO经常咨询怎么换友链，我相信许多新手也会有这些问题。今天我分享友情链接交换的方法是什么，友情链接交换的平台是什么？','友链交换方法及友链交换平台有哪些？','1','/static/upload/2022/01/20/202201201293.jpg','
<p>一些SEO经常咨询怎么换友链，我相信许多新手也会有这些问题。今天我分享友情链接交换的方法是什么，友情链接交换的平台是什么？</p><p><br/></p><p>友情链接交换的方法是什么？</p><p><br/></p><p>
    方法1。对于QQ群来说，更换朋友是很常见的，但与彼此的站长交流非常有效和快捷。</p><p>您可以去QQ群找到自己，并与合适的网站交流。或者把你想交流的网站和一些基本信息链接起来。</p><p><br/></p><p>
    方法2。好友链平台本身可以在互联网上找到好友链平台，查看其他站长发布的信息，如果觉得合适的话可以更换。</p><p><br/></p><p>
    方法3。在自己的网站上添加一个网站，看看彼此的网站上添加了哪些朋友链接。可以直接去网站找站长交流，有些网站有在线客服，遇到好的客服你很幸运，点低吗？让我们换一个。如果你有客户服务，就可以了。你们中的一些人会留下网站管理员的联系信息，并有申请朋友链接的功能。推荐阅读(深圳搜索引擎优化培训)</p>
<p><br/></p><p>方法4。注册更多的网站和论坛，为发布好友链找到一个特殊的目录区域，并把自己</p><p>
    编辑后的网站好友链接信息应该随时发布。建议一些网站不能留下链接，所以不要留下它们，避免删除它们，并留下您的网站名称。关键词相关性，我们应该对这个频道感到满意，哈哈。留下你的微信</p><p>
    一些QQ论坛仍然可以使用。百度贴吧是一部精彩的作品。让我们说点别的。</p><p><br/></p><p>方法5。购买好友链具有选择自由、重量大、流量大的优点。缺点:花钱多、长期提款多、流动性和风险不稳定。</p><p><br/>
</p><p>什么是友好的友情链接交换平台？</p><p><br/></p><p>滴滴友情链接网</p><p><br/></p><p>
    专为站长和SEOer交流和交易友好链接而开发的友好链接平台欢迎站长使用犀牛云链接。它拥有丰富的资源和交易，帮助您节省链交换时间和提高工作效率。</p><p><br/></p><p>云链接链接交换平台</p><p><br/>
</p>','1642640580','0','3','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('5','什么是网站子域名对优化排名优缺点有哪些','8', NULL,'znxw', NULL,'对于刚刚接触到网站建设优化的网站管理员搜索引擎优化人员来说，他们可能不熟悉子域子目录，有些人可能没有听说过。今天将告诉你什么是子域子目录，优化排名的优缺点是什么？希望能够帮助那些新手站长搜索引擎优化人...','dasdsadsa','1','/static/upload/2022/01/19/202201192968.jpg','
<p>对于刚刚接触到网站建设优化的网站管理员搜索引擎优化人员来说，他们可能不熟悉子域子目录，有些人可能没有听说过。今天将告诉你什么是子域子目录，优化排名的优缺点是什么？</p><p><br/></p><p>
    希望能够帮助那些新手站长搜索引擎优化人员！</p><p><br/></p><p>什么是子域？</p><p><br/></p><p>
    子域名称(或子域；中文:子域)是域名系统级别中属于更高级别域的域。例如，mail.example.com和calendar.example.com是example.com的两个子域，而example.com是顶级域的子域。请访问。</p>
<p><br/></p><p>子域名:它是顶级域名的下一级(一级域名或父域名)。域名作为一个整体包括两个。或者包括一个”和一个“/”。</p><p><br/></p><p>什么是子目录？</p><p><br/></p><p>
    子目录:父目录中的目录。子目录也可以有子目录，子目录是无限的。推荐阅读(为什么不去网站关键词排名)</p><p><br/></p><p>
    我们说简单地理解它是在我们网站的根目录下建立的任何文件夹(我们公司用户网站的根目录是wwwroot，如果一个文件夹是在wwwroot下建立为abc，那么abc就是一个子目录，也就是说，这个子目录的名称是abc)。子目录技术可以将任何附加域放在这些任意建立的子目录下。</p>
<p><br/></p><p>子域有什么优点？</p><p><br/></p><p>
    1.使用过网站的人非常清楚，如果域名包含关键词，优化他们的网站是非常有帮助的。当列表出现时，如果你想提高你的瓶颈，你需要搜索引擎所需的权重。当然，这对小站长来说更难。如果我们使用子域，您的列表将翻倍。</p><p><br/></p>
<p>2.他的体重比目录重很多倍。经过仔细优化后，您的权重比子目录更容易获得排名。可以分成很多很多分类页面，也可以单独转移到服务器上，目录无法实现。</p><p><br/></p><p>
    3.如果大量的二级域名组成一个子域站组，这将大大有助于提升主域名的权重。</p><p><br/></p><p>4.该网站规模很大，获得了更多的选票。你也可以挂更多的友情链接。强大的品牌建设。推荐关注点(搜索引擎优化开始)</p>
<p><br/></p><p>子域有什么缺点？</p><p><br/></p><p>1.他最大的重量是他不能继承头版的重量，就像一个新网站一样。如果你的主域优化得很好，你可以考虑打开另一个网站。</p><p><br/></p><p>
    2.工作量将会增加，这与主站的内容不协调。内容差异很大，不相关。</p><p><br/></p><p>3.不要滥用子域。搜索引擎很容易将他们视为作弊。最好不要启用没有很多内容的子域。</p><p><br/></p><p>
    子目录有什么优点？</p><p><br/></p><p>子目录的优点是它们可以继承主域的权重。子目录。内容的质量会影响你网站的整体得分。为了更快的操作，只需要一个后台。</p><p><br/></p><p>
    子目录的缺点是什么？</p><p><br/></p><p>缺点是收集压力大，搜索次数少。子目录不利于良好的友谊链接。</p><p><br/></p><p>
    然而，如何使用它取决于您的应用程序对象。如果你想创建一个子域，首先要注意你的网站是否适合这样做。如果你是一个大网站，比如58、Ganji、腾讯、网易、新浪，你自然会选择子域，因为你的规模已经达到了一定的水平，这些网站的信息和资源是巨大而广泛的。使用单个子域名制作一个领域和行业的内容没有问题，也不需要坚持使用单个关键词，而是要考虑用户的习惯。</p>
<p><br/></p><p>
    如果您是一个中小型网站，此时不建议使用子域。我建议使用子目录更合适，因为子域相当于全新的网站，短期内不会给你带来高搜索引擎优化性能。此外，您自己的中小型网站的内容数据相对较少，不足以支持子域的数量。此外，子域增加了维护成本和工作量。如果你没有足够的诗句来管理，你的体重会导致水平的现象。</p>
<p><br/></p>','1642641743','0','5','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('9','极致CMS不使用伪静态可以吗？','5', NULL,'faq', NULL,'不可以！极致CMS必须使用伪静态。因为在自定义链接上面做了一定的处理，所以必须使用伪静态。','极致CMS不使用伪静态可以吗？','1', NULL,'
<p>
    不可以！极致CMS必须使用伪静态。因为在自定义链接上面做了一定的处理，所以必须使用伪静态。</p>','1642943414','0','0','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('10','极致CMS框架是ThinkPHP吗？','5', NULL,'faq', NULL,'不是！是自主研发的FrPHP框架，仓库地址：https://gitee.com/Cherry_toto/FrPHP 也是免费使用的一个简单框架，只不过伪静态配置跟thinkphp一样。','极致CMS框架是ThinkPHP吗？','1', NULL,'
<p>不是！是自主研发的FrPHP框架，仓库地址：<a href="https://gitee.com/Cherry_toto/FrPHP">https://gitee.com/Cherry_toto/FrPHP</a></p><p>
    也是免费使用的一个简单框架，只不过伪静态配置跟thinkphp一样。</p>','1642943502','0','58','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('11','平台靠什么盈利？是否会一直维护下去？','5', NULL,'faq', NULL,'目前平台是不盈利运营的，当然，说不盈利只不过是指不从极致CMS系统授权方面，而官方有应用市场（https://app.jizhicms.cn），还有极致云（https://idc.jizhicms.com/）平台，虽然两个平台没什么收入，但也算盈利的一...','平台靠什么盈利？是否会一直维护下去？','1', NULL,'
<p>目前平台是不盈利运营的，当然，说不盈利只不过是指不从极致CMS系统授权方面，而官方有应用市场（<a href="https://app.jizhicms.cn">https://app.jizhicms.cn</a>），还有极致云（https://idc.jizhicms.com/）平台，虽然两个平台没什么收入，但也算盈利的一部分。我只不过不希望从授权上盈利，我觉得开源免费就应该完全免费，让大家都能安心的用系统。
</p><p>只要不是特殊原因，我们都会一直维护下去。后续我们也会开发更多的产品，给大家使用。</p><p><br/>
</p>','1642943612','0','11','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('7','新手零基础学SEO难吗？','9', NULL,'xyxw', NULL,'许多初学者学习搜索引擎优化大多是新手，新手学习搜索引擎优化更担心的问题是零基本搜索引擎优化难，零基本搜索引擎优化要学多久？今天，我想和你谈谈这个饥饿的问题。我希望我能帮助你！','新手零基础学SEO难吗？','1','/static/upload/2022/01/19/202201198505.jpg','
<p>许多初学者学习搜索引擎优化大多是新手，新手学习搜索引擎优化更担心的问题是零基本搜索引擎优化难，零基本搜索引擎优化要学多久？今天，我想和你谈谈这个饥饿的问题。我希望我能帮助你！</p><p><br/></p><p>
    零基础研究的搜索引擎优化困难吗？</p><p><br/></p><p>
    零基础的定义:零基础意味着搜索引擎优化一无所知。你经常浏览网页吗？你知道搜索引擎的投标位置和共同位置吗？你了解搜索引擎优化的基本定义和功能吗？或者你知道程序，但不知道搜索引擎优化？回答完这些问题后，我们可以定位自己，并确定我们是否真的从零开始。</p>
<p><br/></p><p>
    搜索引擎优化对零基础研究来说困难吗？事实上，这是一个错误的命题。大多数人认为搜索引擎优化很难学，因为学习搜索引擎优化的过程并不难，但大多数人仍然可以做得很好。认为搜索引擎优化不难学，已经在学搜索引擎优化，并且对搜索引擎优化有着深刻的理解，能够正确操作。</p>
<p><br/></p><p>零基础搜索引擎优化研究需要多长时间？</p><p><br/></p><p>
    准备加入搜索引擎优化专业的人，会考虑搜索引擎优化学习多长时间的实际问题，实际上学习搜索引擎优化多长时间是一个错误的命题。学习是无止境的，搜索引擎优化是一个持续的学习过程，几乎没有终点。再说，什么是“会议”？有几个层次。搜索引擎优化的基本学习时间大约是两个月，这两个月的每一天都需要固定的时间来学习和记忆。</p>
<p><br/></p><p>成为搜索引擎优化专家需要多长时间？</p><p><br/></p><p>
    答案是不确定的。没有必要花很多时间成为搜索引擎优化行业的大玩家。除了必要的时间投入，大量的独立思考，大量的实际网站优化，大量的参考优化技术和个人知识库的存储都是必要的。</p><p><br/></p><p>
    搜索引擎优化学习的基本内容是什么？</p><p><br/></p><p>1.关键词:分析、挖掘、密度分析、布局、查询和排名、长尾关键词排名</p><p><br/></p><p>
    2.站内优化:站内优化细节、网址优化、伪静态和动态、死链接查询和解决方案、链内优化、网站地图制作、301重定向设置、robots.txt、404错误页面设置、图片优化技术、网站内容策略。</p><p><br/></p><p>
    3.站外优化:站外优化策略、友谊链接交换、网站提交门户、站外软文章推广和高质量的站外链架设。</p><p><br/>
</p>','1642645778','0','87','1','0','0','0','0','SEO','0', NULL, NULL,',3,', NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('8','极致CMS免费吗？','5', NULL,'faq', NULL,'极致CMS完全免费，不收取系统的授权费，且免费商用！但是，有的模板如果有标记授权或者付费的，另当别论，因为有的模板二开过，有些文件在系统内，可能整站出售。','极致CMS免费吗？','1', NULL,'
<p>极致CMS完全免费，不收取系统的授权费，且免费商用！</p><p>
    但是，有的模板如果有标记授权或者付费的，另当别论，因为有的模板二开过，有些文件在系统内，可能整站出售。</p>','1642943290','0','32','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('6','网站死链接应该如何处理','8', NULL,'znxw', NULL,'网站死链接应该如何处理？当你的网站有了死链接之后，你觉得这些链接对网站的影响不是很大，那么就可以删除这些链接，但是在删除的过程中一定要注意不能扩大化删除链接，即删除链接不要太多，将死链接删除即可。而遇...','网站死链接应该如何处理','1','/static/upload/2022/01/19/202201199543.jpg','
<p>
    网站死链接应该如何处理？当你的网站有了死链接之后，你觉得这些链接对网站的影响不是很大，那么就可以删除这些链接，但是在删除的过程中一定要注意不能扩大化删除链接，即删除链接不要太多，将死链接删除即可。而遇到一些栏目链接的时候一定不要将之删除了，那样会造成网站栏目的缺乏，站在用户体验的角度上来讲无法对用户构成利好，可能用户这次进入了你的网站，下次看到你的网站就会直接关闭，不能够形成用户二次访问率。</p>
<p><br/></p><p>转化页面，当你检查到网站的页面不存在的时候就可以设置转化页面进行转化，不过这个过程比较复杂，如果网站的死链接过多，那么对站长的技术与耐心也是一个挑战。</p><p><br/></p><p>
    利用屏蔽方法，所谓的屏蔽方法就是通过robots.txt文档将死链接屏蔽掉，让搜索引擎蜘蛛不能发现这些链接的存在，从搜索引擎的角度来讲这个方法是有效的，它逃避了搜索引擎的处罚。但是这个方法却不能够长期的使用，因为这样会造成整个网站垃圾信息过多，还不如通过404引导页面进行引导点击，这种效果对用户与搜索引擎都友好。</p>
<p><br/></p><p>错误链接处理方法：</p><p><br/></p><p>
    当网站打不开的时候先不要着急，看看是否是你输入错误。最好的解决方法就是细心。由此导弹总结以下两点希望各位站长注意：总体来说死链接和错误链接都是一样的，都是打不开的页面。工作中多多细心尽量减少死链接页面，因为死链接越多对你的优化和用户体验越不利。减少客户操作失误造成的错误链接，最好的办法就是用一个简单易记，方便输入的域名。域名中最好不要带符号等。</p>
<p><br/></p><p>首先要明确自己建站的目的是什么?</p><p><br/></p><p>
    网站类型非常之多，每一种类型其建站的目的都是不一致的，我们以企业网站为例子，企业网站无疑就是品牌产品宣传的主要阵地，主要需要面度的是定向客户因为行业不一致导致用户细分也是有差异的，在比如一个个人站长资讯类站点挂百度或者谷歌广告，当然流量为王，越多的访问量才能增加广告被点击的几率可见两者的目的有着质的区别，这一点大家一定要弄清楚。</p>
<p><br/></p><p>网站上线之前的栏目策划是必须的。</p><p><br/></p><p>
    我们知道，一个裸站是没有任何价值和意义的，网站新上线我们首先要进行外包装，比如LOGO，网站导航的设置，网站界面美工的优化，然后针对自己站点的用户需求和关键词的竞争度分析，仔细做好网站栏目的添加，这些细节全部设置好之后，我们就要针对网站内容进行关键词的布局，合理的规划首页布局尽可能的囊括所有SEO优化细节。</p>
<p><br/></p><p>网站上线之后第一时间提交各大搜索引擎。</p><p><br/></p><p>
    作为一个站长，网站建设并不是我们的最终目的，我们的核心是关注网站的权重和排名，seo优化的思维是贯穿在整个网站建设的过程中的，网站上线之后一个非常重要的细节就是尽快提交至各大搜索引擎，提交之前我们要注意一个细节要点，就是尽可能的为网站更新2篇到三篇文章，文章一定要注意质量，新站上线一定要为用户和搜索引擎留下良好的第一印象才对，这个时候我们就可以登陆各大搜索引擎的提交入口了，接下来的工作我就不用在赘述了吧!内容、外链、关键词分析布局，坚持就这样把这些基础工作做精做细吧!</p>
<p><br/></p>','1642644634','0','88','1','0','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0');
INSERT INTO `jz_article` (`id`,`title`,`tid`,`molds`,`htmlurl`,`keywords`,`description`,`seo_title`,`userid`,`litpic`,`body`,`addtime`,`orders`,`hits`,`isshow`,`comment_num`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`) VALUES ('12','网站SEO优化需要原创内容吗？','9','article','xyxw','seo', NULL,'网站SEO优化需要原创内容吗？','0','/static/upload/2022/01/26/202201267840.jpg','
<p>随着互联网发展的日渐成熟，越来越多人选择做百度自然排名SEO，那么，有很多人就疑惑了，网站SEO优化需要原创内容吗？答案是肯定的，原因如下：</p><p><br/></p><p>　　1、百度蜘蛛更喜欢原创文章</p><p><br/>
</p><p>
    　　原创文章很重要，特别是对于新站来说，因为百度蜘蛛对于抄袭的文章会厌恶，如果百度蜘蛛在爬取你的网站的时候，发现你的网站的内容是以前网上发布过的，那么收录你的网站的文章的可能性就会变得极低，百度蜘蛛不收录，你的网站被客户搜索到的几率也就会大大降低，当客户都没办法搜索到你的网站的时候，网站建设也就没有意义可言了。所以企业网站SEO优化的时候，一定要坚持写原创文章，而且需要持续性的更新，这虽然是一个漫长的过程，但是却非常重要。</p>
<p><br/></p><p>
    　　原创的内容，不仅受百度蜘蛛喜爱，也更受客户欢迎。如果客户在浏览你的网站的时候，发现你网站的内容是网上没有过的，比较新颖的，也会特别关注你的网站，浏览的时间也就会更长，交易的可能性也就越高。当客户都喜欢网站的内容的时候，搜索引擎也会了解到这一点，在爬取的时候也会收录更多的内容。所以，坚持原创内容的更新，是一个良性循环、一举两得的事情。</p>
<p><br/></p><p>网站质量</p><p><br/></p><p>　　2、内容原创很重要，价值更重要</p><p><br/></p><p>
    　　原创文章对于企业网站SEO优化是很重要，但是原创文章就一定可以吗?答案也是否定的，在做内容的时候，我们也不能因为原创而原创。很多站长为了原创，利用一些伪原创工具制造文章更新，这么做文章的原创度是提高了，但是内容却没有任何价值可言，没有价值的内容，不仅百度蜘蛛不会喜欢，用户更不会喜欢，长此以往，百度蜘蛛也就不会再来爬取你的站点了，收录只会越来越少。所以，站长在做企业网站的时候，万不能为了原创而丢了质量，要坚持带给客户原创，坚持文章内容的可读性，坚持具有价值的内容，牺牲网站内容质量去迎合百度蜘蛛是非常愚蠢的行为。</p>
<p><br/></p><p>　　3、伪原创是否可行?</p><p><br/></p><p>
    　　在某种意义上来说，伪原创也是可行的，转载也可以。但是一定要注意比例，原创文章为主，伪原创转载类文章为辅。如果伪原创或者转载的内容是质量高的，对用户非常有价值的，那么你的文章哪怕不是原创，百度蜘蛛也会很喜欢，会收录。当然，哪怕是伪原创或者转载的内容，也需要注意跟网站的相关性，不能随意转载。</p>
<p><br/></p><p>　　原创内容确实很重要，但是也需要掌握原创的方法，注重营销型网站内容的内容建设，重视内容的可读性，不能为了迎合百度蜘蛛忽略了用户，搜索引擎是根本，用户体验是未来发展的方向，二者不是独立的，需要相辅相成。</p>
<p><br/></p><p>　　以上就是《网站SEO优化需要原创内容吗？》的全部内容，仅供站长朋友们互动交流学习，SEO优化是一个需要坚持的过程，希望大家一起共同进步。</p><p><br/>
</p>','1643161498','0','9','0','0','0','0','0', NULL,'1', NULL, NULL, NULL, NULL,'0');
-- ----------------------------
-- Records of jz_attr
-- ----------------------------
INSERT INTO `jz_attr` (`id`,`name`,`isshow`) VALUES ('1','置顶','1');
INSERT INTO `jz_attr` (`id`,`name`,`isshow`) VALUES ('2','热点','1');
INSERT INTO `jz_attr` (`id`,`name`,`isshow`) VALUES ('3','推荐','1');
-- ----------------------------
-- Records of jz_buylog
-- ----------------------------
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('1','0','1','No20220123161635','3','jifen','登录奖励', NULL,'1.00','1.00','1642925795');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('2','9','0','No20220123220149','3','jifen','点赞奖励','product','1.00','1.00','1642946509');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('3','9','0','No20220123220206','3','jifen','取消点赞','product','-1.00','-1.00','1642946526');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('4','9','0','No20220123220209','3','jifen','点赞奖励','product','1.00','1.00','1642946529');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('5','9','0','No20220123220358','3','jifen','取消点赞','product','-1.00','-1.00','1642946638');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('6','10','0','No20220123220410','3','jifen','点赞奖励','product','1.00','1.00','1642946650');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('7','9','0','No20220123220413','3','jifen','点赞奖励','product','1.00','1.00','1642946653');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('8','8','0','No20220123220415','3','jifen','点赞奖励','product','1.00','1.00','1642946655');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('9','9','0','No20220123220441','3','jifen','收藏奖励','product','1.00','1.00','1642946681');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('10','9','0','No20220123220450','3','jifen','取消收藏','product','-1.00','-1.00','1642946690');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('11','9','0','No20220123220450','3','jifen','取消收藏','product','-1.00','-1.00','1642946690');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('12','9','0','No20220123220655','3','jifen','收藏奖励','product','1.00','1.00','1642946815');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('13','10','0','No20220123220726','3','jifen','收藏奖励','product','1.00','1.00','1642946846');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('14','7','0','No20220123220730','3','jifen','收藏奖励','product','1.00','1.00','1642946850');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('15','10','0','No20220123221918','3','jifen','取消收藏','product','-1.00','-1.00','1642947558');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('16','10','0','No20220123221918','3','jifen','取消收藏','product','-1.00','-1.00','1642947558');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('17','10','0','No20220123221923','3','jifen','收藏奖励','product','1.00','1.00','1642947563');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('18','9','0','No20220123224512','3','jifen','取消收藏','product','-1.00','-1.00','1642949112');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('19','9','0','No20220123224512','3','jifen','取消收藏','product','-1.00','-1.00','1642949112');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('20','10','0','No20220123224623','3','jifen','取消点赞','product','-1.00','-1.00','1642949183');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('21','0','1','No20220125083513','3','jifen','登录奖励', NULL,'1.00','1.00','1643070913');
INSERT INTO `jz_buylog` (`id`,`aid`,`userid`,`orderno`,`type`,`buytype`,`msg`,`molds`,`amount`,`money`,`addtime`) VALUES ('22','0','1','No20220126083805','3','jifen','登录奖励', NULL,'1.00','1.00','1643157485');
-- ----------------------------
-- Records of jz_cachedata
-- ----------------------------
-- ----------------------------
-- Records of jz_chain
-- ----------------------------
-- ----------------------------
-- Records of jz_classtype
-- ----------------------------
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('1','公司产品','公司产品','product', NULL, NULL, NULL, NULL,'0','1','1','1','0','0','product','list','details','9','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('2','公司新闻','公司新闻','article', NULL, NULL, NULL, NULL,'0','1','1','1','0','0','news','article-list','article-details','10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('3','关于我们','关于我们','page', NULL, NULL, NULL,'
<p>极致CMS是一款免费开源的建站系统。具备基础的CMS模型，可以使用它进行发布更新，进行内容管理。</p><p>而除此之外，它还有会员模块，支付模块，积分模块，不仅拥有丰富的插件，还可以自由扩展。</p><p>
    简单的模板标签，使你能够更快更便捷的建站。强大的拓展性，让它能够胜任市场上绝大多数的功能开发。</p><p>
    具备良好的SEO优化，更容易被浏览器抓取收录。毫秒级响应，百万数据承载，对大数据处理有非常丰富的经验。</p>','0','1','1','0','0','0','about-us','about-us','article-details','10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('4','联系我们','联系我们','message','/static/cms/static/images/jizhicms.jpg', NULL, NULL, NULL,'0','1','1','0','0','0','contact','contact-us', NULL,'10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('5','常见问题','常见问题','article', NULL, NULL, NULL, NULL,'0','1','1','0','0','0','faq','faq','article-details','10','0', NULL,'0','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('6','免费模板','免费模板','product', NULL, NULL, NULL, NULL,'0','1','1','0','1','0','free','list','details','9','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('7','商业模板','商业模板','product', NULL, NULL, NULL, NULL,'0','1','1','0','1','0','business','list','details','9','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('8','站内新闻','站内新闻','article', NULL, NULL, NULL, NULL,'0','1','1','0','2','0','znxw','article-list','article-details','10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('9','行业新闻','行业新闻','article', NULL, NULL, NULL, NULL,'0','1','1','0','2','0','xyxw','article-list','article-details','10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('10','用户评价','用户评价','pingjia', NULL, NULL, NULL, NULL,'0','1','0','0','0','0','yhpj','lists','details','10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('11','技术支持','技术支持','page', NULL, NULL, NULL, NULL,'0','1','1','0','3','0','support','page','details','10','0','https://www.jizhicms.cn','1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('12','隐私协议','隐私协议','page', NULL, NULL, NULL,'
<p><span style="color: rgb(255, 0, 0);"><strong>极致CMS遵循 MIT协议！</strong></span></p><p><span
            style="color: rgb(255, 0, 0);"><strong>除此外，极致CMS不允许做违纪违法的事情，如果有参与，一律自行负责，与极致CMS无关！</strong></span>
</p>','0','1','1','0','3','0','privacy-agreement','page','details','10','0', NULL,'1','0', NULL);
INSERT INTO `jz_classtype` (`id`,`classname`,`seo_classname`,`molds`,`litpic`,`description`,`keywords`,`body`,`orders`,`orderstype`,`isshow`,`iscover`,`pid`,`gid`,`htmlurl`,`lists_html`,`details_html`,`lists_num`,`comment_num`,`gourl`,`ishome`,`isclose`,`gids`) VALUES ('13','广告合作','广告合作','page', NULL, NULL, NULL,'
<p style="white-space: normal;">极致CMS由开发者&nbsp;<strong>留恋风（如沐春）[ 25841047041@qq.com ]</strong>&nbsp;独立开发完成！</p><p
        style="white-space: normal;">Gitee ：<a href="https://gitee.com/Cherry_toto/jizhicms">https://gitee.com/Cherry_toto/jizhicms</a>
</p><p style="white-space: normal;">Github ：<a href="https://github.com/Cherry-toto/jizhicms">https://github.com/Cherry-toto/jizhicms</a>
</p><p style="white-space: normal;"><br/></p><p style="white-space: normal;">邮箱 ：2581047041@qq.com</p><p
        style="white-space: normal;">QQ ：2581047041</p><p style="white-space: normal;">微信 ：TF-2581047041</p><p
        style="white-space: normal;"><br/></p><p style="white-space: normal;"><span
            style="color: rgb(255, 0, 0);"><strong>极致CMS源码不会携带任何广告，如果有广告合作的朋友，暂时不会在系统上加，十分抱歉！</strong></span></p><p
        style="white-space: normal;">可以通过邮箱或者QQ联系（由于提问的人数过多，建议发送邮件咨询，QQ已加爆！<img
            src="http://img.baidu.com/hi/jx2/j_0012.gif"/>），另外主要在QQ交流群内解答问题，如果你有问题，可以到QQ群里来提问。</p><p><br/></p><p>
    如果有其他项目合作的朋友，随时可以添加QQ微信联系我！<br/>
</p>','0','1','1','0','3','0','cooperation','page','details','10','0', NULL,'1','0', NULL);
-- ----------------------------
-- Records of jz_collect
-- ----------------------------
-- ----------------------------
-- Records of jz_collect_type
-- ----------------------------
-- ----------------------------
-- Records of jz_comment
-- ----------------------------
INSERT INTO `jz_comment` (`id`,`tid`,`aid`,`pid`,`zid`,`body`,`reply`,`addtime`,`userid`,`likes`,`isshow`,`isread`) VALUES ('1','8','6','0','0','很不错！', NULL,'1642931294','1','0','1','0');
INSERT INTO `jz_comment` (`id`,`tid`,`aid`,`pid`,`zid`,`body`,`reply`,`addtime`,`userid`,`likes`,`isshow`,`isread`) VALUES ('2','8','6','1','0',' @iPHfa6 干得漂亮！', NULL,'1642932172','1','0','1','0');
INSERT INTO `jz_comment` (`id`,`tid`,`aid`,`pid`,`zid`,`body`,`reply`,`addtime`,`userid`,`likes`,`isshow`,`isread`) VALUES ('3','6','10','0','0','很不错哦！', NULL,'1643167629','1','0','1','0');
-- ----------------------------
-- Records of jz_ctype
-- ----------------------------
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('1','基本设置','base',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('2','高级设置','high-level',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('3','搜索配置','searchconfig',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('4','邮件订单','email-order',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('5','支付配置','payconfig',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('6','公众号配置','wechatbind',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('7','积分配置','jifenset',1,1);
INSERT INTO `jz_ctype` (`id`,`title`,`action`,`sys`,`isopen`) VALUES ('8','图片水印','imagewatermark',1,1);
-- ----------------------------
-- Records of jz_customurl
-- ----------------------------
-- ----------------------------
-- Records of jz_fields
-- ----------------------------
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('1','url','links','链接地址', NULL,'1',',0,','255', NULL,'0','1','1','1','0','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('2','title','links','链接名称', NULL,'1', NULL,'255', NULL,'1','1','1','1','1','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('3','email','message','联系邮箱', NULL,'1', NULL,'255', NULL,'0','0','1','1','1','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('4','keywords','tags','关键词','尽量简短，但不能重复','1', NULL,'50', NULL,'0','1','1','1','1','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('5','newname','tags','替换词','尽量简短，但不能重复，20字以内，可不填。【已废弃】','1', NULL,'50', NULL,'0','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('7','num','tags','替换次数','一篇文章内替换的次数，默认-1，全部替换【已废弃】','4', NULL,'4', NULL,'0','0','1','0','0','0', NULL,'-1','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('8','target','tags','打开方式', NULL,'7', NULL,'50','新窗口=_blank,本窗口=_self','0','0','1','0','0','0', NULL,'_blank','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('9','number','tags','标签数','无需填写，程序自动处理','4', NULL,'11', NULL,'0','0','1','1','0','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('10','member_id','article','用户','前台会员，无需填写','15', NULL,'11','3,username','0','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('11','member_id','product','用户','前台会员，无需填写','15', NULL,'11','3,username','0','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('12','member_id','links','发布用户','前台会员，无需填写','13', NULL,'11','3,username','0','0','0','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('13','target','links','外链URL','默认为空，系统访问内容则直接跳转到此链接','1', NULL,'255', NULL,'0','0','0','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('14','ownurl','links','自定义URL','默认为空，自定义URL','1', NULL,'255', NULL,'0','0','0','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('15','ownurl','tags','自定义URL','默认为空，自定义URL','1', NULL,'255', NULL,'0','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('16','addtime','links','添加时间','系统自带','11', NULL,'11', NULL,'0','0','0','0','0','0','date_2','0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('17','addtime','tags','添加时间','系统自带','11', NULL,'11', NULL,'0','0','1','1','0','0','date_2','0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('43','molds','product','模型', NULL,'15', NULL,'50', NULL,'1','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('19','title','article','标题', NULL,'1', NULL,'255', NULL,'1','1','1','1','1','1', NULL, NULL,'1','0','0','250','1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('20','tid','article','所属栏目', NULL,'17', NULL,'13', NULL,'1','1','1','1','1','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('21','molds','article','模型', NULL,'15', NULL,'50', NULL,'1','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('22','htmlurl','article','栏目链接', NULL,'1', NULL,'255', NULL,'1','0','1','0','0','0', NULL, NULL,'1','0','1', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('23','keywords','article','关键词', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('24','description','article','简介', NULL,'2', NULL,'0', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('25','seo_title','article','SEO标题', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('26','userid','article','管理员', NULL,'15', NULL,'11','11,name','1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('27','litpic','article','缩略图', NULL,'5', NULL,'255', NULL,'1','0','1','1','0','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('28','body','article','内容', NULL,'3', NULL,'0', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('29','addtime','article','发布时间', NULL,'11',NULL,'11', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0','150','0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('30','orders','article','排序', NULL,'4', NULL,'4', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('31','hits','article','点击量', NULL,'4', NULL,'11', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('32','isshow','article','是否显示', NULL,'7',',0,','1','显示=1,未审=0,退回=2','1','0','1','1','1','1', NULL,'1','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('33','comment_num','article','评论数', NULL,'4', NULL,'11', NULL,'1','0','1','0','0','0', NULL,'0','1','0','1', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('34','istop','article','是否置顶：1是0否', NULL,'1',',0,1,2,3,4,5,6,7,8,9,10,11,12,13,','2','是=1,否=0','1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('35','ishot','article','是否头条：1是0否', NULL,'1',',0,1,2,3,4,5,6,7,8,9,10,11,12,13,','2','是=1,否=0','1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('36','istuijian','article','是否推荐：1是0否', NULL,'1',',0,1,2,3,4,5,6,7,8,9,10,11,12,13,','2','是=1,否=0','1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('37','tags','article','Tags', NULL,'19',',0,1,2,3,4,5,6,7,8,9,10,11,12,13,','255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('38','target','article','外链', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','1', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('39','ownurl','article','自定义链接', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','1', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('40','jzattr','article','推荐属性', NULL,'16', NULL,'255','14,name','1','0','1','1','1','1', NULL, NULL,'1','0','0','150','0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('41','tids','article','副栏目', NULL,'18', NULL,'255', NULL,'100','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('42','zan','article','点赞数', NULL,'4', NULL,'11', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('44','title','product','标题', NULL,'1', NULL,'255', NULL,'1','1','1','1','1','1', NULL, NULL,'1','100','0','300','1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('45','seo_title','product','SEO标题', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('46','tid','product','所属栏目', NULL,'17', NULL,'11', NULL,'1','0','1','1','1','1', NULL,'0','1','100','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('47','hits','product','点击量', NULL,'4',',0,10,','11', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('48','htmlurl','product','栏目链接', NULL,'1', NULL,'255', NULL,'1','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('49','keywords','product','关键词', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('50','description','product','简介', NULL,'2', NULL,'0', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('51','litpic','product','缩略图', NULL,'5', NULL,'255', NULL,'1','0','1','1','0','1', NULL, NULL,'1','100','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('52','stock_num','product','库存', NULL,'1', NULL,'11', NULL,'1','0','1','1','0','0', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('53','price','product','价格', NULL,'1', NULL,'10,2', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('54','pictures','product','图集', NULL,'6',',0,1,2,3,4,5,6,7,8,9,10,11,12,13,', NULL, NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('55','isshow','product','是否显示', NULL,'7',',0,','1','显示=1,未审=0,退回=2','1','0','1','1','0','1', NULL,'1','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('56','comment_num','product','评论数', NULL,'4', NULL,'11', NULL,'1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('57','body','product','内容', NULL,'3', NULL,'0', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('58','userid','product','管理员', NULL,'15', NULL,'11','11,name','1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('59','orders','product','排序', NULL,'4', NULL,'4', NULL,'1','0','1','1','0','1', NULL,'0','1','100','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('60','addtime','product','发布时间', NULL,'11',NULL,'11', NULL,'1','0','1','1','0','1', NULL,'0','1','99','0','120','0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('61','istop','product','是否置顶：1是0否', NULL,'1', NULL,'2', NULL,'1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('62','ishot','product','是否头条：1是0否', NULL,'1', NULL,'2', NULL,'1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('63','istuijian','product','是否推荐：1是0否', NULL,'1', NULL,'2', NULL,'1','0','1','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('64','tags','product','Tags', NULL,'19', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('65','target','product','外链', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('66','ownurl','product','自定义链接', NULL,'1', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('67','jzattr','product','推荐属性', NULL,'16', NULL,'255','14,name','1','0','1','1','1','1', NULL, NULL,'1','100','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('68','tids','product','副栏目', NULL,'18', NULL,'255', NULL,'1','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('69','zan','product','点赞数', NULL,'4', NULL,'11', NULL,'1','0','1','1','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('70','isshow','tags','是否显示', NULL,'7', NULL,'1','显示=1,隐藏=0,退回=2','0','0','1','1','1','1', NULL,'1','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('71','lx','product','类型', NULL,'7',',1,6,7,','2','响应式=1,PC=2,手机=3,PC+手机=4,小程序=5','2','0','1','1','1','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('72','color','product','颜色', NULL,'7',',1,6,7,','2','红色=1,橙色=2,黄色=3,绿色=4,蓝色=5,紫色=6,粉色=7','2','0','1','1','1','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('73','hy','product','行业', NULL,'8',',1,6,7,','500','金融/证券=1,IT科技/软件=2,教育/培训=3,珠宝/工艺品=4,五金/机电=5,婚庆/摄影/美容=6,旅游/餐饮/美食=7,房产/汽车/运输=8,休闲/文化=9,医疗/生物/化工=10,儿童/游乐园=11,动物/宠物=12,鲜花/礼物=13,运动/俱乐部=14,生态/农业=15,建筑/装饰=16,广告/网站/设计=17,个人/导航/博客=18','2','0','1','1','1','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('74','title','pingjia','用户名','默认为空','1',',0,10,','255', NULL,'100','0','1','1','1','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('75','tid','pingjia','所属栏目','选择栏目','17',',10,','11', NULL,'100','0','1','1','1','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('76','tids','pingjia','副栏目','绑定后可以在当前模型的其他栏目中显示','18', NULL,'255', NULL,'100','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('77','keywords','pingjia','关键词','每个词用英文逗号(,)拼接','1', NULL,'255', NULL,'100','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('78','tags','pingjia','TAG','每个词用英文逗号(,)拼接','19', NULL,'255', NULL,'100','0','1','0','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('79','litpic','pingjia','头像','可留空','5',',0,10,','255', NULL,'100','0','1','1','0','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('80','description','pingjia','简述','可留空','2',',0,10,','500', NULL,'100','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('81','body','pingjia','内容','可留空','3',',10,','500', NULL,'100','0','1','1','0','0', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('82','member_id','pingjia','发布会员','前台发布会员ID记录','13', NULL,'11','3,username','100','0','0','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('83','userid','pingjia','管理员','后台发布管理员ID记录','13', NULL,'11','11,name','100','0','0','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('84','target','pingjia','外链URL','默认为空，系统访问内容则直接跳转到此链接','1', NULL,'255','11,name','100','0','0','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('85','ownurl','pingjia','自定义URL','默认为空，自定义URL','1', NULL,'255','11,name','100','0','0','0','0','0', NULL, NULL,'1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('86','hits','pingjia','点击量','系统自动添加','4', NULL,'11', NULL,'100','0','0','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('87','comment_num','pingjia','评论数','系统自带','4', NULL,'11', NULL,'100','0','0','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('88','zan','pingjia','点赞数','系统自带','4', NULL,'11', NULL,'100','0','0','0','0','0', NULL,'0','1','0','0', NULL,'0');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('89','addtime','pingjia','添加时间','选择时间','11',',10,','11', NULL,'100','0','1','1','0','1','date_2','0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('90','jzattr','pingjia','推荐属性','1置顶2热点3推荐','16', NULL,'50','14,name','100','0','1','0','0','0', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('91','isshow','pingjia','是否显示','显示隐藏','7',',10,','1','显示=1,隐藏=0,退回=2','100','0','1','1','1','1', NULL,'1','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('92','zhiye','pingjia','职业', NULL,'1',',10,','255', NULL,'100','0','1','1','0','1', NULL, NULL,'1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES ('93','orders','pingjia','排序', NULL,'4', NULL,'4', NULL,'1','0','1','1','0','1', NULL,'0','1','0','0', NULL,'1');
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (94, 'username', 'member', '用户昵称', NULL, 1, ',0,', '255', NULL, 2, 1, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (95, 'openid', 'member', '微信OPENID', NULL, 1, ',0,', '255', NULL, 2, 0, 1, 1, 0, 1, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (96, 'sex', 'member', '性别', NULL, 12, ',0,', '2', '男=1,女=2,未知=0', 2, 0, 1, 1, 1, 1, NULL, '0', 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (97, 'gid', 'member', '会员分组', NULL, 13, ',0,', '11', '6,name', 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (98, 'litpic', 'member', '会员头像', NULL, 5, ',0,', '255', NULL, 2, 0, 1, 1, 0, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (99, 'tel', 'member', '电话号码', NULL, 1, ',0,', '12', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (100, 'jifen', 'member', '积分', NULL, 14, ',0,', '10,2', NULL, 2, 0, 1, 1, 0, 1, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (101, 'money', 'member', '金币', NULL, 14, ',0,', '10,2', NULL, 2, 0, 1, 1, 0, 1, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (102, 'email', 'member', '邮箱', NULL, 1, ',0,', '255', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (103, 'province', 'member', '省份', NULL, 1, ',0,', '50', NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (104, 'city', 'member', '城市', NULL, 1, ',0,', '50', NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (105, 'address', 'member', '详细地址', NULL, 1, ',0,', '255', NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (106, 'regtime', 'member', '注册时间', NULL, 11, ',0,', '11', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (107, 'logintime', 'member', '最近登录', NULL, 11, ',0,', '11', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (108, 'signature', 'member', '个性签名', NULL, 1, ',0,', '255', NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (109, 'birthday', 'member', '生日', NULL, 1, ',0,', '50', NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (110, 'pid', 'member', '推荐人', NULL, 13, ',0,', '11', '3,username', 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (111, 'isshow', 'member', '状态', '封禁后不能登录', 7, ',0,', '2', '正常=1,封禁=0', 2, 0, 1, 1, 1, 1, NULL, '1', 1, 0, 0, NULL, 0);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (112, 'title', 'message', '标题', NULL, 1, ',4,', '255', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (113, 'user', 'message', '用户昵称', NULL, 1, ',4,', '255', NULL, 2, 0, 1, 0, 1, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (114, 'tid', 'message', '相关栏目', NULL, 13, ',4,', '11', '2,classname', 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (115, 'tel', 'message', '联系电话', NULL, 1, ',4,', '20', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (116, 'ip', 'message', '留言IP', NULL, 1, ',4,', '50', NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (117, 'body', 'message', '留言内容', NULL, 3, ',4,', NULL, NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (118, 'isshow', 'message', '是否审核', NULL, 7, ',4,', '1', '未审核=0,已审核=1', 2, 0, 1, 1, 1, 1, NULL, '0', 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (119, 'addtime', 'message', '提交时间', NULL, 11, ',4,', '11', NULL, 2, 0, 1, 1, 1, 1, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (120, 'reply', 'message', '回复留言', NULL, 3, ',4,', NULL, NULL, 2, 0, 1, 1, 0, 0, NULL, NULL, 1, 0, 0, NULL, 1);
INSERT INTO `jz_fields` (`id`,`field`,`molds`,`fieldname`,`tips`,`fieldtype`,`tids`,`fieldlong`,`body`,`orders`,`ismust`,`isshow`,`isadmin`,`issearch`,`islist`,`format`,`vdata`,`isajax`,`listorders`,`isext`,`width`,`ishome`) VALUES (121, 'uploadsize', 'member', '上传限制', '单位M，上传总文件大小限制，超过此大小不允许上传', 4, ',0,', '11', NULL, 2, 0, 0, 1, 0, 0, NULL, '0', 1, 0, 0, NULL, 0);
-- ----------------------------
-- Records of jz_hook
-- ----------------------------
-- ----------------------------
-- Records of jz_layout
-- ----------------------------
INSERT INTO `jz_layout` (`id`,`name`,`top_layout`,`left_layout`,`gid`,`ext`,`sys`,`isdefault`) VALUES ('1','系统默认','[]','[{"name":"内容管理","icon":"&amp;#xe6b4;","nav":[{"key":"16948","title":"内容列表","value":"9","icon":""},{"key":"12349","title":"商品列表","value":"105","icon":""},{"key":"19748","title":"推荐属性","value":"202","icon":""}]},{"name":"栏目管理","icon":"&amp;#xe699;","nav":[{"key":"10518","title":"栏目列表","value":"42","icon":""}]},{"name":"互动管理","icon":"&amp;#xe69b;","nav":[{"key":"11832","title":"留言列表","value":"22","icon":""},{"key":"11262","title":"评论列表","value":"16","icon":""}]},{"name":"SEO设置","icon":"&amp;#xe6b3;","nav":[{"key":"16628","title":"TAG列表","value":"147","icon":""},{"key":"16214","title":"友情链接","value":"95","icon":""},{"key":"16254","title":"网站地图","value":"153","icon":""},{"key":"16917","title":"内链列表","value":"210","icon":""}]},{"name":"用户管理","icon":"&amp;#xe6b8;","nav":[{"key":"11957","title":"会员列表","value":"2","icon":""},{"key":"15086","title":"会员分组","value":"118","icon":""},{"key":"10618","title":"会员权限","value":"123","icon":""},{"key":"17578","title":"管理员列表","value":"54","icon":""},{"key":"19552","title":"角色管理","value":"49","icon":""},{"key":"10895","title":"权限列表","value":"66","icon":""},{"key":"12582","title":"订单列表","value":"129","icon":""},{"key":"17076","title":"充值列表","value":"177","icon":""}]},{"name":"系统设置","icon":"&amp;#xe6ae;","nav":[{"key":"11314","title":"网站设置","value":"40","icon":""},{"key":"10572","title":"桌面设置","value":"70","icon":""},{"key":"18242","title":"导航设置","value":"190","icon":""},{"key":"13002","title":"轮播图","value":"83","icon":""},{"key":"15936","title":"轮播图分类","value":"89","icon":""},{"key":"19847","title":"清理缓存","value":"114","icon":""},{"key":"12739","title":"模板列表","value":"223","icon":""},{"key":"127391","title":"配置栏目","value":"240","icon":""}]},{"name":"扩展管理","icon":"&amp;#xe6ce;","nav":[{"key":"11957","title":"插件列表","value":"76","icon":""},{"key":"13870","title":"图库管理","value":"116","icon":""},{"key":"12472","title":"模型列表","value":"61","icon":""},{"key":"15551","title":"数据库备份","value":"35","icon":""},{"key":"16311","title":"碎片化","value":"194","icon":""},{"key":"18982","title":"公众号菜单","value":"141","icon":""},{"key":"14568","title":"公众号素材","value":"142","icon":""},{"key":"13219","title":"模板制作","value":"143","icon":""},{"key":"17893","title":"生成静态文件","value":"154","icon":""},{"key":"16926","title":"登录日志","value":"115","icon":""}]},{"name":"回收站","icon":"&amp;#xe8a3;","nav":[{"key":"17056","title":"回收站","value":"217","icon":""}]},{"name":"评价管理","icon":"&amp;#xe717;","nav":[{"key":"16835","title":"用户评价","value":"227","icon":""}]}]','0','CMS默认配置，不可删除！','1','1');
INSERT INTO `jz_layout` (`id`,`name`,`top_layout`,`left_layout`,`gid`,`ext`,`sys`,`isdefault`) VALUES ('2','旧版桌面','[]','[{"name":"网站管理","icon":"&amp;#xe699;","nav":["42","9","95","83","147","22"]},{"name":"商品管理","icon":"&amp;#xe698;","nav":["105","129","2","118","123","16","177"]},{"name":"扩展管理","icon":"&amp;#xe6ce;","nav":["76","116","141","142","143","194","35","61","154","153"]},{"name":"系统设置","icon":"&amp;#xe6ae;","nav":["40","54","49","190","70","115","114","66"]}]','0','旧版本配置','0','0');
-- ----------------------------
-- Records of jz_level
-- ----------------------------
INSERT INTO `jz_level` (`id`,`name`,`pass`,`tel`,`gid`,`email`,`regtime`,`logintime`,`status`) VALUES ('1','admin','0acdd3e4a8a2a1f8aa3ac518313dab9d','13600136000','1','123456@qq.com','1635997469','1643156842','1');
-- ----------------------------
-- Records of jz_level_group
-- ----------------------------
INSERT INTO `jz_level_group` (`id`,`name`,`isadmin`,`ischeck`,`classcontrol`,`paction`,`tids`,`isagree`,`description`) VALUES ('1','超级管理员','1','0','0',',Fields,', NULL,'1', NULL);
-- ----------------------------
-- Records of jz_likes
-- ----------------------------
INSERT INTO `jz_likes` (`id`,`tid`,`aid`,`userid`,`addtime`) VALUES ('4','6','9','1','1642946653');
INSERT INTO `jz_likes` (`id`,`tid`,`aid`,`userid`,`addtime`) VALUES ('5','7','8','1','1642946655');
-- ----------------------------
-- Records of jz_link_type
-- ----------------------------
INSERT INTO `jz_link_type` (`id`,`name`,`addtime`) VALUES ('1','首页','1642818560');
-- ----------------------------
-- Records of jz_links
-- ----------------------------
INSERT INTO `jz_links` (`id`,`title`,`molds`,`url`,`isshow`,`tid`,`userid`,`htmlurl`,`orders`,`member_id`,`target`,`ownurl`,`addtime`) VALUES ('1','极致CMS','links','https://www.jizhicms.cn','1','1','1', NULL,'0','0', NULL, NULL,'0');
INSERT INTO `jz_links` (`id`,`title`,`molds`,`url`,`isshow`,`tid`,`userid`,`htmlurl`,`orders`,`member_id`,`target`,`ownurl`,`addtime`) VALUES ('2','极致应用市场','links','https://app.jizhicms.cn','1','1','1', NULL,'0','0', NULL, NULL,'0');
-- ----------------------------
-- Records of jz_member
-- ----------------------------
INSERT INTO `jz_member` (`id`,`username`,`openid`,`pass`,`token`,`sex`,`gid`,`litpic`,`tel`,`jifen`,`likes`,`collection`,`money`,`email`,`address`,`province`,`city`,`regtime`,`logintime`,`isshow`,`signature`,`birthday`,`follow`,`fans`,`ismsg`,`iscomment`,`iscollect`,`islikes`,`isat`,`isrechange`,`pid`) VALUES ('1','极致用户', NULL,'1321321321312', NULL,'0','1','/static/upload/user/head_1.jpeg','13600136000','3.00', NULL, NULL,'0.00', NULL, NULL, NULL, NULL,'1642925638','1643157485','1', NULL, NULL, NULL,'0','1','1','1','1','1','1','0');
-- ----------------------------
-- Records of jz_member_group
-- ----------------------------
INSERT INTO `jz_member_group` (`id`,`name`,`description`,`paction`,`pid`,`isagree`,`iscomment`,`ischeckmsg`,`addtime`,`orders`,`discount`,`discount_type`) VALUES ('1','注册会员','前台会员分组，最低等级分组',',Message,Comment,User,Order,Home,Common,Uploads,','0','1','1','1','0','0','0.00','0');
-- ----------------------------
-- Records of jz_menu
-- ----------------------------
-- ----------------------------
-- Records of jz_message
-- ----------------------------
INSERT INTO `jz_message` (`id`,`title`,`userid`,`tid`,`aid`,`user`,`ip`,`body`,`tel`,`addtime`,`orders`,`email`,`isshow`,`istop`,`hits`,`tids`) VALUES ('1','联系我们','0','0','0','测试客户','127.0.0.1','
<p>这是一条测试留言</p>','13600136000','1643100950','0','123456@qq.com','0','0','0', NULL);
INSERT INTO `jz_message` (`id`,`title`,`userid`,`tid`,`aid`,`user`,`ip`,`body`,`tel`,`addtime`,`orders`,`email`,`isshow`,`istop`,`hits`,`tids`) VALUES ('2','联系我们','0','0','0','测试123','127.0.0.1','
<p>这是一条测试留言</p>','13600136000','1643102345','0','2311232131@qq.com','0','0','0', NULL);
-- ----------------------------
-- Records of jz_molds
-- ----------------------------
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('1','内容','article','1','1','1','1','1','1','article-list.html','article-details.html','100','0','1');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('2','栏目','classtype','1','1','1','1','1','1','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('3','会员','member','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('4','订单','orders','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('5','商品','product','1','1','1','1','1','1','list.html','details.html','100','0','1');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('6','会员分组','member_group','1','1','0','0','1','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('7','评论','comment','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('8','留言','message','1','1','0','0','1','1','message.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('9','轮播图','collect','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('10','友情链接','links','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('11','管理员','level','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('12','TAG','tags','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('13','单页','page','1','1','1','1','1','1','page.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('14','推荐属性','attr','1','1','0','0','0','0','list.html','details.html','100','1','0');
INSERT INTO `jz_molds` (`id`,`name`,`biaoshi`,`sys`,`isopen`,`iscontrol`,`ismust`,`isclasstype`,`isshowclass`,`list_html`,`details_html`,`orders`,`ispreview`,`ishome`) VALUES ('15','用户评价','pingjia','0','1','0','1','1','1','lists.html','details.html','100','0','0');
-- ----------------------------
-- Records of jz_orders
-- ----------------------------
INSERT INTO `jz_orders` (`id`,`orderno`,`userid`,`paytype`,`ptype`,`tel`,`username`,`tid`,`price`,`jifen`,`qianbao`,`body`,`receive_username`,`receive_tel`,`receive_email`,`receive_address`,`ispay`,`paytime`,`addtime`,`send_time`,`isshow`,`discount`,`yunfei`) VALUES ('1','No20220125084425','1', NULL,'1','13600136000','iPHfa6','0','0.01','1.00','0.01','||7-6-1-0.01||', NULL, NULL, NULL, NULL,'0','0','1643071465','0','1','0.00','0.00');
INSERT INTO `jz_orders` (`id`,`orderno`,`userid`,`paytype`,`ptype`,`tel`,`username`,`tid`,`price`,`jifen`,`qianbao`,`body`,`receive_username`,`receive_tel`,`receive_email`,`receive_address`,`ispay`,`paytime`,`addtime`,`send_time`,`isshow`,`discount`,`yunfei`) VALUES ('2','No20220125151109','1', NULL,'1','13600136000','iPHfa6','0','0.02','2.00','0.02','||7-7-2-0.01||', NULL, NULL, NULL, NULL,'0','0','1643094669','0','1','0.00','0.00');
-- ----------------------------
-- Records of jz_page
-- ----------------------------
-- ----------------------------
-- Records of jz_pictures
-- ----------------------------
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('1','1','0','product','Admin','jpg','14.24','/static/upload/2022/01/19/202201199543.jpg','1642592754','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('2','1','0','product','Admin','jpg','17.91','/static/upload/2022/01/19/202201194641.jpg','1642593917','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('3','1','0','product','Admin','jpg','12.47','/static/upload/2022/01/19/202201198505.jpg','1642594016','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('4','1','0','product','Admin','jpg','10.41','/static/upload/2022/01/19/202201192886.jpg','1642594063','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('5','1','0','product','Admin','jpg','11.62','/static/upload/2022/01/19/202201192968.jpg','1642594125','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('6','8','0','article','Admin','png','79.87','/static/upload/2022/01/20/202201202799.png','1642639629','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('7','8','0','article','Admin','jpg','9.66','/static/upload/2022/01/20/202201202461.jpg','1642639668','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('8','0','0', NULL,'Admin','jpeg','122.31','/static/upload/image/20220120/1642640161966799.jpeg','1642640161','0');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('9','8','0','article','Admin','jpg','13.05','/static/upload/2022/01/20/202201206807.jpg','1642640183','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('10','8','0','article','Admin','jpg','17.91','/static/upload/2022/01/20/202201209692.jpg','1642640284','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('11','9','0','article','Admin','jpg','11.62','/static/upload/2022/01/20/202201201293.jpg','1642640663','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('12','10','0','pingjia','Admin','jpeg','22.38','/static/upload/2022/01/20/202201207970.jpeg','1642678774','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('13','10','0','pingjia','Admin','jpeg','34.07','/static/upload/2022/01/20/202201202736.jpeg','1642678847','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('14','10','0','pingjia','Admin','jpeg','25.34','/static/upload/2022/01/20/202201207507.jpeg','1642679235','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('15','10','0','pingjia','Admin','jpeg','19.63','/static/upload/2022/01/20/202201209411.jpeg','1642679469','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('16','10','0','pingjia','Admin','jpeg','25.94','/static/upload/2022/01/20/202201201541.jpeg','1642679928','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('17','10','0','pingjia','Admin','jpeg','18.82','/static/upload/2022/01/20/202201205173.jpeg','1642680404','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('18','10','0','pingjia','Admin','jpeg','16.53','/static/upload/2022/01/22/202201226081.jpeg','1642817000','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('19','6','0','product','Admin','jpg','11.43','/static/upload/2022/01/24/202201248147.jpg','1643024022','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('20','6','0','product','Admin','jpg','11.04','/static/upload/2022/01/24/202201248943.jpg','1643024022','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('21','6','0','product','Admin','jpg','17.91','/static/upload/2022/01/24/202201244087.jpg','1643024023','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('22','6','0','product','Admin','jpg','14.24','/static/upload/2022/01/24/202201247813.jpg','1643024023','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('23','9','0','article','Home','jpg','16.44','/static/upload/2022/01/26/202201267840.jpg','1643161485','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('24','6','0','product','Home','jpg','41.2','/static/upload/2022/01/26/202201263577.jpg','1643161953','1');
INSERT INTO `jz_pictures` (`id`,`tid`,`aid`,`molds`,`path`,`filetype`,`size`,`litpic`,`addtime`,`userid`) VALUES ('27','0','0','member','Home','jpeg','30.93','/static/upload/user/head_1.jpeg','1643163524','1');
-- ----------------------------
-- Records of jz_pingjia
-- ----------------------------
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('1','10', NULL,'杜*小','/static/upload/2022/01/20/202201207970.jpeg', NULL,'简单、方便，还免费！','
<p>
    相比于其他的cms，极致CMS就特别简洁，后台进去没有什么多余的广告信息，清爽简洁！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642678180','个人博主');
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('2','10', NULL,'慕*文','/static/upload/2022/01/20/202201202736.jpeg', NULL,'功能强大，免费开源，搞钱神器！','
<p>
    找了很多网上的“免费”CMS，除了免费给你看，其他很多都要付费，后台各种广告，随便找个插件都要钱！而且还不能去版权，必须挂到主页底下，自从看到极致CMS后我再也不用担心了，出了功能强大之外，免费开源，想改哪里就改哪里，主页还不用挂版权，真的是业界良心！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642678780','自由职业');
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('3','10', NULL,'王*鑫','/static/upload/2022/01/20/202201207507.jpeg', NULL,'开源免费！这个CMS是真的开源免费！','
<p>
    不说别人，就开源免费而言，就甩其他同类CMS几条街！什么是开源？有的CMS还加密一些文件。极致CMS不仅免费，而且各个地方都可以自由定义，真的做到了自主自由！群主还经常在群里热心回答，帮助我很多，非常感谢！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642679206','互联网小白');
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('4','10', NULL,'张小姐','/static/upload/2022/01/20/202201209411.jpeg', NULL,'群主热心，挺不错的cms！','
<p>
    刚接触cms时，群主远程手把手帮我安装，虽然我很笨，但是群里好多热心人帮我，帮我解决了一个大难题！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642679458','网站运营');
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('5','10', NULL,'程*安','/static/upload/2022/01/20/202201201541.jpeg', NULL,'好用，方便，简单。越用越是觉得这个CMS的强大！非常棒！','
<p>
    之前朋友推荐给我的这个CMS，当时还是觉得小众，感觉所有的cms都一个样。后面因为要做个站，就选用这个cms，不得不说一开始确实一脸懵逼，特别是他的逻辑跟织梦这些有区别，但是想法却很新颖。后面陆陆续续做了几个站，看了群主的视频教程，对cms也比较了解了，现在随便一个功能型的站，我都能用极致做出来！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642679894','SEO');
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('7','10', NULL,'吴*强','/static/upload/2022/01/22/202201226081.jpeg', NULL,'开源免费，容易搞钱！','
<p>
    没有用极致CMS之前，都觉得网上的CMS基本上是简单的功能，要功能就得付费，而且对于不懂程序的人而言，那是相当难，自从用了极致之后，你会觉得每一天都在成长，能力越来越强，可以用它来做任何系统！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642816961','个人站长');
INSERT INTO `jz_pingjia` (`id`,`tid`,`tids`,`title`,`litpic`,`keywords`,`description`,`body`,`molds`,`userid`,`orders`,`member_id`,`comment_num`,`htmlurl`,`isshow`,`target`,`ownurl`,`jzattr`,`hits`,`zan`,`tags`,`addtime`,`zhiye`) VALUES ('6','10', NULL,'梁*宽','/static/upload/2022/01/20/202201205173.jpeg', NULL,'群主好人，免费开源，还经常回答问题！','
<p>
    个人觉得cms对新手有一定难度，不过当你真正做了几个站之后，你就会发现这个CMS是真的强大，每当我觉得做不出来功能的时候，群里一问，里面就有群友出一些解决方案，而且自己能完成！对于互联网小白而言，这个是不可思议的，因为我没学过编程，但完成了一个其他CMS需要花很多钱二开的功能！那一刻，很自豪！</p>','pingjia','1','0','0','0', NULL,'1', NULL, NULL,'0','0','0', NULL,'1642680280','博客小新人');
-- ----------------------------
-- Records of jz_plugins
-- ----------------------------
-- ----------------------------
-- Records of jz_power
-- ----------------------------
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('1','Common','公共权限','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('2','Home','前台网站','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('3','User','个人中心','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('4','Login','会员登录','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('5','Message','站内留言','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('6','Comment','会员评论','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('7','Screen','网站筛选','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('8','Order','会员下单','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('9','Mypay','网站支付','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('10','Jzpay','极致支付','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('11','Tags','TAG标签','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('12','Wechat','微信模块','0','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('13','Common/vercode','验证码生成','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('14','Common/checklogin','检查是否登录','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('15','Common/multiuploads','多附件上传','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('16','Common/uploads','单附件上传','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('17','Common/qrcode','二维码生成','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('18','Common/get_fields','获取扩展信息','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('19','Common/jizhi','链接错误提示','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('20','Common/error','报错提示','1','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('21','Home/index','网站首页','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('22','Home/jizhi','网站内容','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('23','Home/auto_url','自定义链接','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('24','Home/jizhi_details','详情内容','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('25','Home/search','网站搜索','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('26','Home/searchAll','网站多模块搜索','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('27','Home/start_cache','开启网站缓存','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('28','Home/end_cache','输出缓存','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('29','User/checklogin','检查是否登录','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('30','User/index','个人中心首页','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('31','User/userinfo','会员资料','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('32','User/orders','订单记录','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('33','User/orderdetails','订单详情','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('34','User/payment','订单支付','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('35','User/orderdel','删除订单','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('36','User/changeimg','上传头像','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('37','User/comment','评论列表','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('38','User/commentdel','删除评论','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('39','User/likesAction','点赞文章','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('40','User/likes','点赞列表','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('41','User/likesdel','取消点赞','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('42','User/collectAction','收藏文章','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('43','User/collect','收藏列表','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('44','User/collectdel','删除收藏','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('45','User/cart','购物车','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('46','User/addcart','添加购物车','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('47','User/delcart','删除购物车','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('48','User/posts','发布管理','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('49','User/release','会员发布','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('50','User/del','删除发布','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('51','User/uploads','会员上传附件','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('52','User/jizhi','404提示','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('53','User/follow','关注用户','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('54','User/nofollow','取消关注','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('55','User/fans','粉丝列表','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('56','User/notify','消息提醒','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('57','User/notifyto','查看消息','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('58','User/notifydel','删除消息','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('59','User/active','公共主页','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('60','User/setmsg','消息提醒设置','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('61','User/getclass','获取栏目列表','2','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('62','User/wallet','用户钱包','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('63','User/buy','会员充值','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('64','User/buylist','充值列表','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('65','User/buydetails','交易详情','3','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('66','Login/index','登录首页','4','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('67','Login/register','注册页面','4','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('68','Login/forget','忘记密码','4','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('69','Login/nologin','未登录页面','4','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('70','Login/loginout','退出登录','4','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('71','Message/index','发送留言','5','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('72','Comment/index','发表评论','6','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('73','Screen/index','筛选列表','7','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('74','Order/create','创建订单','8','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('75','Order/pay','订单支付','8','1');
INSERT INTO `jz_power` (`id`,`action`,`name`,`pid`,`isagree`) VALUES ('76','Tags/index','TAG标签列表','11','1');
-- ----------------------------
-- Records of jz_product
-- ----------------------------
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('1','product','PC端橙色IT科技教育培训网站模板','PC端橙色IT科技教育培训网站模板','6','1','free', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201194641.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL,',2,', NULL,'0','2','2',',2,3,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('2','product','响应式红色软件公司网站模板','响应式红色软件公司网站模板','6','0','free', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201199543.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0','1','1',',1,2,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('3','product','手机端黄色五金机电网站模板','手机端黄色五金机电网站模板','6','0','free', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201198505.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL,',3,', NULL,'0','3','3',',4,5,6,7,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('4','product','PC+手机绿色医疗生物化工网站模板','PC+手机绿色医疗生物化工网站模板','6','0','free', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201192886.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0','4','4',',10,11,12,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('5','product','蓝色小程序鲜花礼物广告设计网站模板','蓝色小程序鲜花礼物广告设计网站模板','7','0','business', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201192968.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL,',2,', NULL,'0','5','5',',2,13,14,15,16,17,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('6','product','PC端橙色IT科技教育培训网站模板','PC端橙色IT科技教育培训网站模板','7','2','business', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201194641.jpg','99','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL,',3,', NULL,'0','2','2',',2,3,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('7','product','响应式红色软件公司网站模板','响应式红色软件公司网站模板','7','0','business', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201199543.jpg','98','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL,',2,2,', NULL,'0','1','1',',1,2,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('8','product','手机端黄色五金机电网站模板','手机端黄色五金机电网站模板','7','1','business', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201198505.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL,',3,', NULL,'0','3','3',',4,5,6,7,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('9','product','PC+手机绿色医疗生物化工网站模板','PC+手机绿色医疗生物化工网站模板','6','0','free', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201192886.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0','4','4',',10,11,12,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('10','product','蓝色小程序鲜花礼物广告设计网站模板','蓝色小程序鲜花礼物广告设计网站模板','6','0','free', NULL,'响应式网站模板源码自适应，同一个后台，数据即时同步，简单适用！附带测试数据！友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！后台：域名/admin.p...','/static/upload/2022/01/19/202201192968.jpg','100','0.01','/static/upload/2022/01/24/202201248147.jpg|A||/static/upload/2022/01/24/202201248943.jpg|B||/static/upload/2022/01/24/202201244087.jpg|C||/static/upload/2022/01/24/202201247813.jpg|D','1','0','
<p>响应式网站模板源码</p><p><br/></p><p>自适应，同一个后台，数据即时同步，简单适用！附带测试数据！</p><p>
    友好的seo，所有页面均都能完全自定义标题/关键词/描述，PHP程序，安全、稳定、快速；用低成本获取源源不断订单！</p><p><br/></p><p>后台：域名/admin.php</p><p>账号：admin</p><p>
    密码：admin</p><p><br/></p><p>使用教程：xxxxx</p><p><br/></p><p>模板特点</p><p>1：手工书写DIV+CSS、代码精简无冗余。</p><p>
    2：自适应结构，全球先进技术，高端视觉体验。</p><p>3：SEO框架布局，栏目及文章页均可独立设置标题/关键词/描述。</p><p>4：附带测试数据、安装教程、入门教程、安全及备份教程。</p><p>
    5：后台直接修改联系方式、传真、邮箱、地址等，修改更加方便。</p><p><br/></p><p>语言程序：PHP + SQLite</p><p>前端规范：html+css+jQuery</p><p>设备支持：PC端+手机端</p>
<p>浏览器支持：兼容IE7+、Firefox、Chrome、360浏览器等主流浏览器</p><p>最佳分辨率：1920px+1440px</p><p>程序运行环境：linux+nginx/ linux+apache / windows +
    iis(支持php5.3+) / 其他支持php5.3+环境</p><p><br/>
</p>','1','0','1642592648','0','0','0', NULL,'0', NULL, NULL, NULL, NULL,'0','5','5',',2,13,14,15,16,17,');
INSERT INTO `jz_product` (`id`,`molds`,`title`,`seo_title`,`tid`,`hits`,`htmlurl`,`keywords`,`description`,`litpic`,`stock_num`,`price`,`pictures`,`isshow`,`comment_num`,`body`,`userid`,`orders`,`addtime`,`istop`,`ishot`,`istuijian`,`tags`,`member_id`,`target`,`ownurl`,`jzattr`,`tids`,`zan`,`lx`,`color`,`hy`) VALUES ('11','product','响应式蓝色软件博客模板','响应式蓝色软件博客模板','6','3','free', NULL,'免费响应式蓝色软件博客模板','/static/upload/2022/01/26/202201263577.jpg','10','0.00', NULL,'1','0','
<p>1、安装教程：极致CMS网站安装教程</p><p><br/></p><p>2、网站安全设置教程：极致CMS网站安全设置教程</p><p><br/></p><p>3、非模板BUG修改另付费</p><p><br/></p><p>
    4、模板BUG修改请直接联系站长，验证信息请填写您的订单号</p><p><br/></p><p>5、不解答有关任何免费模板的问题（解答付费）</p><p><br/></p><p>
    6、后台已配置好，不要乱点，整出些妖蛾子，浪费彼此时间</p><p><br/></p><p>7、缩略图请按照源码示例进行制作，要不然又说图片变形什么的</p><p><br/>
</p>','0','0','1643162020','0','0','0', NULL,'1', NULL, NULL, NULL, NULL,'0','1','5',',2,18,');
-- ----------------------------
-- Records of jz_recycle
-- ----------------------------
-- ----------------------------
-- Records of jz_ruler
-- ----------------------------
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('1','会员管理','Member','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('2','会员列表','Member/index','1','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('3','添加会员','Member/memberadd','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('4','修改会员','Member/memberedit','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('5','删除会员','Member/member_del','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('6','批量删除','Member/deleteAll','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('7','修改状态','Member/change_status','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('8','内容管理','Article','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('9','内容列表','Article/articlelist','8','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('10','添加内容','Article/addarticle','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('11','修改内容','Article/editarticle','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('12','删除内容','Article/deletearticle','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('13','批量删除','Article/deleteAll','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('14','复制内容','Article/copyarticle','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('15','评论管理','Comment','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('16','评论列表','Comment/commentlist','15','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('17','添加评论','Comment/addcomment','15','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('18','修改评论','Comment/editcomment','15','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('19','删除评论','Comment/deletecomment','15','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('20','批量删除','Comment/deleteAll','15','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('21','留言管理','Message','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('22','留言列表','Message/messagelist','21','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('23','修改留言','Message/editmessage','21','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('24','删除留言','Message/deletemessage','21','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('25','批量删除','Message/deleteAll','21','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('26','字段管理','Fields','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('27','字段列表','Fields/index','26','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('28','新增字段','Fields/addFields','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('29','修改字段','Fields/editFields','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('30','删除字段','Fields/deleteFields','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('31','获取字段','Fields/get_fields','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('32','基本功能','Index','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('33','系统界面','Index/index','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('34','后台首页','Index/welcome','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('35','数据库备份','Index/beifen','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('36','数据库备份','Index/backup','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('37','数据库还原','Index/huanyuan','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('38','数据库删除','Index/shanchu','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('39','系统功能','Sys','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('40','网站设置','Sys/index','39','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('41','栏目管理','Classtype','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('42','栏目列表','Classtype/index','41','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('43','新增栏目','Classtype/addclass','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('44','修改栏目','Classtype/editclass','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('45','删除栏目','Classtype/deleteclass','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('46','修改排序','Classtype/editClassOrders','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('47','栏目隐藏','Classtype/change_status','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('48','管理员管理','Admin','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('49','角色管理','Admin/group','48','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('50','新增角色','Admin/groupadd','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('51','修改角色','Admin/groupedit','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('52','删除角色','Admin/group_del','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('53','角色状态','Admin/change_group_status','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('54','管理员列表','Admin/adminlist','48','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('55','新增管理员','Admin/adminadd','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('56','修改管理员','Admin/adminedit','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('57','管理员状态','Admin/change_status','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('58','删除管理员','Admin/admindelete','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('59','个人信息','Index/details','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('60','模型管理','Molds','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('61','模型列表','Molds/index','60','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('62','新增模型','Molds/addMolds','60','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('63','修改模型','Molds/editMolds','60','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('64','删除模型','Molds/deleteMolds','60','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('65','权限管理','Rulers','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('66','权限列表','Rulers/index','65','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('67','新增权限','Rulers/addrulers','65','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('68','修改权限','Rulers/editrulers','65','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('69','删除权限','Rulers/deleterulers','65','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('70','桌面设置','Index/desktop','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('71','新增桌面','Index/desktop_add','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('72','修改桌面','Index/desktop_edit','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('73','删除桌面','Index/desktop_del','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('74','图标库','Index/unicode','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('75','插件管理','Plugins','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('76','插件列表','Plugins/index','75','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('77','模块扩展','Extmolds','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('82','轮播图','Collect','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('83','轮播图','Collect/index','82','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('84','新增轮播图','Collect/addcollect','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('85','修改轮播图','Collect/editcollect','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('86','删除轮播图','Collect/deletecollect','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('87','复制轮播图','Collect/copycollect','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('88','批量删除轮播图','Collect/deleteAll','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('89','轮播图分类','Collect/collectType','82','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('90','新增轮播图分类','Collect/collectTypeAdd','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('91','修改轮播图分类','Collect/collectTypeEdit','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('92','删除轮播图分类','Collect/collectTypeDelete','82','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('93','批量复制','Article/copyAll','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('94','批量修改栏目','Article/changeType','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('95','友情链接','Links/index','189','1','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('96','新增友链','Links/addlinks','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('97','修改友链','Links/editlinks','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('98','复制友链','Links/copylinks','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('99','删除友链','Links/deletelinks','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('100','批量删除友链','Links/deleteAll','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('101','通用模块','Common','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('102','上传文件','Common/uploads','101','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('103','更新cookie','Index/update_session_maxlifetime','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('104','商品管理','Product','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('105','商品列表','Product/productlist','104','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('106','新增商品','Product/addproduct','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('107','修改商品','Product/editproduct','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('108','删除商品','Product/deleteproduct','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('109','复制商品','Product/copyproduct','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('110','批量删除','Product/deleteAll','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('111','批量复制','Product/copyAll','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('112','修改栏目','Product/changeType','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('113','修改排序','Product/editProductOrders','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('114','清理缓存','Index/cleanCache','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('115','登录日志','Sys/loginlog','39','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('116','图库管理','Sys/pictures','39','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('117','修改排序','Extmolds/editOrders','77','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('118','会员分组','Member/membergroup','1','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('119','新增分组','Member/groupadd','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('120','修改分组','Member/groupedit','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('121','更改分组状态','Member/change_group_status','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('122','删除分组','Member/group_del','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('123','会员权限','Member/power','1','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('124','添加权限','Member/addrulers','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('125','修改权限','Member/editrulers','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('126','删除权限','Member/deleterulers','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('127','修改分组排序','Member/editOrders','1','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('128','订单管理','Order','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('129','订单列表','Order/index','128','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('130','订单详情','Order/details','128','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('131','批量删除','Order/deleteAll','128','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('132','上传支付证书','Sys/uploadcert','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('133','更改状态','Plugins/change_status','75','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('134','安装卸载','Plugins/action_do','75','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('223','模板列表','Template/index','222','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('222','模板管理','Template','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('137','删除图库图片','Sys/deletePic','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('138','批量删除图库','Sys/deletePicAll','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('139','安装说明','Plugins/desc','75','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('140','微信公众号','Wechat','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('141','公众号菜单','Wechat/wxcaidan','140','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('142','公众号素材','Wechat/sucai','140','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('143','模板制作','Index/showlabel','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('144','获取首字母拼音','Classtype/get_pinyin','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('145','批量新增栏目','Classtype/addmany','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('146','自定义配置删除','Sys/custom_del','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('147','TAG列表','Extmolds/index/molds/tags','77','1','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('148','新增TAG','Extmolds/addmolds/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('149','修改TAG','Extmolds/editmolds/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('150','复制TAG','Extmolds/copymolds/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('151','删除TAG','Extmolds/deletemolds/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('152','批量删除TAG','Extmolds/deleteAll/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('153','网站地图','Index/sitemap','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('154','生成静态文件','Index/tohtml','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('155','更新栏目HTML','Index/html_classtype','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('156','更新模块HTML','Index/html_molds','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('157','批量修改推荐属性','Article/changeAttribute','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('158','批量修改推荐属性','Product/changeAttribute','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('159','批量修改友链栏目','Links/changeType','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('160','批量修改TAG栏目','Extmolds/changeType/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('161','批量复制友链','Links/copyAll','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('162','批量复制TAG','Extmolds/copyAll/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('163','批量修改友链排序','Links/editOrders','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('164','批量修改TAG排序','Extmolds/editOrders/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('165','删除订单','Order/deleteorder','128','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('166','批量删除','Admin/deleteAll','48','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('167','高级设置','Sys/ctype/type/high-level','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('168','邮箱订单','Sys/ctype/type/email-order','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('169','支付配置','Sys/ctype/type/payconfig','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('170','公众号配置','Sys/ctype/type/wechatbind','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('171','批量审核','Article/checkAll','8','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('172','批量审核','Product/checkAll','104','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('173','批量审核','Message/checkAll','21','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('174','批量审核','Comment/checkAll','15','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('175','批量审核友链','Links/checkAll','189','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('176','批量审核TAG','Extmolds/checkAll/molds/tags','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('177','充值列表','Order/czlist','128','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('178','手动充值','Order/chongzhi','128','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('179','删除记录','Order/delbuylog','128','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('180','批量删除记录','Order/delAllbuylog','128','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('181','积分配置','Sys/ctype/type/jifenset','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('182','插件更新','Plugins/update','75','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('183','获取栏目模板','Classtype/get_html','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('184','批量修改栏目','Classtype/changeClass','41','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('185','友链分类','Links/linktype','189','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('186','新增友链分类','Links/linktypeadd','189','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('187','修改友链分类','Links/linktypeedit','189','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('188','删除友链分类','Links/linktypedelete','189','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('189','友情链接','Links','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('190','导航设置','Index/menu','32','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('191','新增导航','Index/addmenu','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('192','修改导航','Index/editmenu','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('193','删除导航','Index/delmenu','32','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('194','碎片化','Sys/datacache','39','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('195','新增碎片','Sys/addcache','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('196','修改碎片','Sys/editcache','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('197','删除碎片','Sys/delcache','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('198','预览SQL','Sys/viewcache','39','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('199','搜索配置','Sys/ctype/type/searchconfig','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('200','修改字段属性','Fields/editFieldsValue','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('201','推荐属性','Jzattr','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('202','推荐属性','Jzattr/index','201','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('203','新增推荐属性','Jzattr/addAttr','201','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('204','修改推荐属性','Jzattr/editAttr','201','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('205','删除推荐属性','Jzattr/delAttr','201','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('206','修改状态','Jzattr/changeStatus','201','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('207','列表设置','Fields/fieldsList','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('208','获取列表字段','Fields/fieldsList','26','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('209','内链模块','Jzchain','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('210','内链列表','Jzchain/index','209','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('211','新增内链','Jzchain/addchain','209','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('212','修改内链','Jzchain/editchain','209','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('213','删除内链','Jzchain/delchain','209','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('214','批量删除','Jzchain/delAll','209','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('215','修改状态','Jzchain/changeStatus','209','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('216','回收站','Recycle','0','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('217','回收站','Recycle/index','216','1','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('218','恢复数据','Recycle/restore','216','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('219','删除数据','Recycle/del','216','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('220','批量删除','Recycle/delAll','216','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('221','批量恢复','Recycle/restoreAll','216','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('224','安装卸载','Template/actionDo','222','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('225','安装说明','Template/desc','222','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('226','模板更新','Template/update','222','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('227','用户评价列表','Extmolds/index/molds/pingjia','77','1','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('228','新增用户评价','Extmolds/addmolds/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('229','修改用户评价','Extmolds/editmolds/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('230','复制用户评价','Extmolds/copymolds/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('231','删除用户评价','Extmolds/deletemolds/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('232','批量删除用户评价','Extmolds/deleteAll/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('233','批量修改用户评价栏目','Extmolds/changeType/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('234','批量复制用户评价','Extmolds/copyAll/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('235','批量修改用户评价列表','Extmolds/editOrders/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('236','批量审核用户评价','Extmolds/checkAll/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('237','重构字段','Molds/restrucFields','60','0','1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('238','基本配置','Sys/ctype/type/base','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('239','批量修改评价推荐属性','Extmolds/changeAttribute/molds/pingjia','77','0','0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('240','配置栏目','Sys/systype','39',1,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('241','设置配置状态','Sys/systypestatus','39',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('242','修改配置分组','Sys/editctype','39',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('243','新增配置分组','Sys/addctype','39',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('244','全局配置','Sys/ctype','39',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('245','修改配置字段','Sys/setfield','39',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('246','绑定模块数据获取','Fields/getSelect','26',0,'0');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('247','编辑器上传','Uploads','0',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('248','上传功能','Uploads/index','247',0,'1');
INSERT INTO `jz_ruler` (`id`,`name`,`fc`,`pid`,`isdesktop`,`sys`) VALUES ('249','获取子栏目','Classtype/getchildren','41','0','1');
-- ----------------------------
-- Records of jz_shouchang
-- ----------------------------
INSERT INTO `jz_shouchang` (`id`,`tid`,`aid`,`userid`,`addtime`) VALUES ('5','6','10','1','1642947563');
INSERT INTO `jz_shouchang` (`id`,`tid`,`aid`,`userid`,`addtime`) VALUES ('4','7','7','1','1642946850');
-- ----------------------------
-- Records of jz_sysconfig
-- ----------------------------
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('1','web_version','系统版号','版本号是系统自带，请勿改动','0','2.5.2','0', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('2','web_name','网站SEO名称','控制在25个字、50个字节以内','2','极致CMS建站系统','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('3','web_keyword','网站SEO关键词','5个左右，8汉字以内，用英文逗号隔开','2','极致建站,cms,开源cms,免费cms,cms系统,phpcms,免费企业建站,建站系统,企业cms,jizhicms,极致cms,建站cms,建站系统,极致博客,极致blog,内容管理系统','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('4','web_desc','网站SEO描述','控制在80个汉字，160个字符以内','3','极致CMS是开源免费的PHPCMS网站内容管理系统，无商业授权，简单易用，提供丰富的插件，帮您实现零基础搭建不同类型网站（企业站，门户站，个人博客站等），是您建站的好帮手。极速建站，就选极致CMS。','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('5','web_js','统计代码','将百度统计、cnzz等平台的流量统计JS代码放到这里','8', NULL,'1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('6','web_copyright','底部版权','如：&copy; 2016 xxx版权','2','@2020-2099','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('7','web_beian','备案号','如：京ICP备00000000号','2','冀ICP备88888号','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('8','web_tel','网站电话','网站联系电话','2','0666-8888888','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('9','web_tel_400','400电话','400电话','2','400-0000-000','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('10','web_qq','网站QQ','网站QQ','2','12345678','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('11','web_email','网站邮箱','网站邮箱','2','123456@qq.com','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('12','web_address','公司地址','公司地址','2','河北省廊坊市广阳区xxx大厦xx楼001号','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('13','pc_template','PC网站模板','将模板名称填写到此处','2','cms','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('14','wap_template','WAP网站模板','开启了手机端，这个设置才会生效，否则调用电脑端模板','2','1','2',NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('15','weixin_template','微信网站模板','开启了手机端，这个设置才会生效，否则调用电脑端模板。由于微信内有一些特殊的js，所以可以在这里单独设置微信模板','2','cms','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('16','iswap','是否开启手机端','如果不开启手机端，则默认调用电脑端模板','6','1','2','开启=1,关闭=0','1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('17','isopenhomeupload','是否开启前台上传','关闭后，前台无法上传文件。如果网站没有使用会员，建议关闭前台上传。','6','1','2','开启=1,关闭=0','1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('18','isopenhomepower','是否开启前台权限','开启后前台用户权限可以在后台控制','6','0','2','开启=1,关闭=0','1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('19','cache_time','缓存时间','单位：分钟，留空或0则不设置缓存。如果生成静态文件，静态文件清空后才生效。','2','0','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('20','fileSize','限制上传文件大小','0代表不限，单位kb','2','0','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('21','fileType','允许上传文件类型','请用|分割，如：pdf|jpg|png','2','pdf|jpg|jpeg|png|zip|rar|gzip|doc|docx|xlsx','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('22','ueditor_config','后台编辑器导航条配置', "后台UEditor编辑器导航条配置",'3','&quot;fullscreen&quot;, &quot;source&quot;,&quot;undo&quot;, &quot;redo&quot;,&quot;bold&quot;, &quot;italic&quot;, &quot;underline&quot;, &quot;fontborder&quot;, &quot;strikethrough&quot;, &quot;super&quot;, &quot;removeformat&quot;, &quot;formatmatch&quot;, &quot;autotypeset&quot;, &quot;blockquote&quot;, &quot;pasteplain&quot;,&quot;forecolor&quot;, &quot;backcolor&quot;, &quot;insertorderedlist&quot;, &quot;insertunorderedlist&quot;, &quot;selectall&quot;, &quot;cleardoc&quot;,&quot;rowspacingtop&quot;, &quot;rowspacingbottom&quot;, &quot;lineheight&quot;,&quot;customstyle&quot;, &quot;paragraph&quot;, &quot;fontfamily&quot;, &quot;fontsize&quot;,&quot;directionalityltr&quot;, &quot;directionalityrtl&quot;, &quot;indent&quot;,&quot;justifyleft&quot;, &quot;justifycenter&quot;, &quot;justifyright&quot;, &quot;justifyjustify&quot;,&quot;touppercase&quot;, &quot;tolowercase&quot;,&quot;link&quot;, &quot;unlink&quot;, &quot;anchor&quot;, &quot;imagenone&quot;, &quot;imageleft&quot;, &quot;imageright&quot;, &quot;imagecenter&quot;,&quot;simpleupload&quot;, &quot;insertimage&quot;, &quot;emotion&quot;, &quot;scrawl&quot;, &quot;insertvideo&quot;, &quot;music&quot;, &quot;attachment&quot;, &quot;map&quot;, &quot;gmap&quot;, &quot;insertframe&quot;, &quot;insertcode&quot;, &quot;webapp&quot;, &quot;pagebreak&quot;,&quot;template&quot;, &quot;background&quot;,&quot;horizontal&quot;, &quot;date&quot;, &quot;time&quot;, &quot;spechars&quot;, &quot;snapscreen&quot;, &quot;wordimage&quot;,&quot;inserttable&quot;, &quot;deletetable&quot;, &quot;insertparagraphbeforetable&quot;, &quot;insertrow&quot;, &quot;deleterow&quot;, &quot;insertcol&quot;, &quot;deletecol&quot;, &quot;mergecells&quot;, &quot;mergeright&quot;, &quot;mergedown&quot;, &quot;splittocells&quot;, &quot;splittorows&quot;, &quot;splittocols&quot;, &quot;charts&quot;,&quot;print&quot;, &quot;preview&quot;, &quot;searchreplace&quot;, &quot;help&quot;, &quot;drafts&quot;','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('23','search_table','允许前台搜索的表','防止数据泄露,填写允许发布模块标识,留空表示不允许发布,多个表可用|分割,如：article|product','2','article|product','3', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('24','imagequlity','上传图片压缩比例','100%则不压缩，如果PNG是透明图，压缩后背景变黑色。格式如：80','6','75','2','不压缩使用原图=100,95%=95,90%=90,85%=85,80%=80,75%=75,70%=70,65%=65,60%=60,55%=55,50%=50','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('25','ispngcompress','PNG是否压缩','PNG压缩后容易变成背景黑色，关闭后，不会压缩。','6','0','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('26','email_server','邮件服务器','smtp.163.com,smtp.qq.com','2','smtp.163.com','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('27','email_port','邮件收发端口','163、126邮件端口(465)，QQ邮件端口(587)','2','465','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('28','shou_email','收件人Email地址', NULL,'2', NULL,'4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('29','send_email','发件人Email地址','指邮件服务器发件邮箱','2', NULL,'4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('30','send_pass','发件人Email秘钥','这个秘钥不是登录密码','2', NULL,'4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('31','send_name','发件人昵称','发件邮箱会带一个昵称','2','极致建站系统','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('32','tj_msg','客户订单通知','购买商品的时候会发送的一条邮件信息','3','尊敬的{xxx}，我们已经收到您的订单！请留意您的电子邮件以获得最新消息，谢谢您！','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('33','send_msg','订单出货通知','发货的时候发送给客户的通知','3','尊敬的{xxx}，我们已确认了您的订单，请于3日内汇款，逾期恕不保留，不便请见谅。汇款完成后，烦请告知客服人员您的交易账号后五位，即完成下单手续，谢谢您。','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('34','yunfei','订单运费','购物下单时会加上这个运费','2','0.00','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('35','paytype','在线支付','0关闭支付，1自主平台支付','6','0','5','关闭=0,开启=1','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('40','alipay_partner','支付宝APPID','账户中心->密钥管理->开放平台密钥，填写添加了电脑网站支付的应用的APPID','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('41','alipay_key','支付宝key','MD5密钥，安全检验码，由数字和字母组成的32位字符串','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('42','alipay_private_key','支付宝私钥', NULL,'3', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('43','alipay_public_key','支付宝公钥', NULL,'3', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('44','wx_mchid','微信商户mchid','支付相关','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('45','wx_key','微信商户key','支付相关','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('46','wx_appid','微信公众号appid','支付相关','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('47','wx_appsecret','微信公众号appsecret','支付相关','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('48','wx_client_cert','微信apiclient_cert','支付相关','5', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('49','wx_client_key','微信apiclient_key','支付相关','5', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('50','wx_login_appid','公众号appid','用户登录相关，如果跟支付的一样，那就再填写一遍','2', NULL,'6', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('51','wx_login_appsecret','公众号appsecret','用户登录相关，如果跟支付的一样，那就再填写一遍','2', NULL,'6', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('52','wx_login_token','公众号token','用户登录相关，如果跟支付的一样，那就再填写一遍','2', NULL,'6', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('53','huanying','公众号关注欢迎语','公众号关注时发送的第一句推送','3','欢迎关注公众号~','6', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('54','wx_token','公众号token','支付相关','2', NULL,'5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('55','web_logo','网站LOGO', NULL,'1','/static/cms/static/images/logo.png','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('56','admintpl','后台模板风格','内页弹窗：点击新增/修改等操作，页面是一个弹出层，更美观。内嵌页面：点击新增/修改等操作，页面直接进入新页面，不会弹出层。','6','default','2','内页弹窗=default,内嵌页面=tpl','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('59','domain','网站SEO网址','一般不填，全局网址，最后不带/,如：http://www.xxx.com','2', NULL,'1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('61','overtime','订单超时','按小时计算，超过该小时订单过期，仅限于开启支付后，0代表不限制','2','4','4', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('62','islevelurl','开启层级URL','默认关闭层级URL，开启后URL会按照父类层级展现','6','0','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('63','iscachepage','缓存完整页面','前台完整页面缓存，结合缓存时间，可以提高访问速度','6','1','0','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('64','isautohtml','自动生成静态','前台访问网站页面，将自动生成静态HTML，下次访问直接进入静态HTML页面','0','0','0','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('65','pc_html','PC静态文件目录','电脑端静态HTML存放目录，默认根目录[ / ]','2','/','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('66','mobile_html','WAP静态文件目录','手机端静态HTML存放目录，默认[ m ]，PC和WAP静态目录不能相同，否则文件会混乱','2','m','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('67','autocheckmessage','是否留言自动审核','开启后，留言自动审核（显示）','6','0','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('68','autocheckcomment','是否评论自动审核','开启后评论自动审核（显示）','6','1','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('69','mingan','网站敏感词过滤','将敏感词放到里面，用“,”分隔，用{xxx}代替通配内容','3', NULL,'1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('70','iswatermark','是否开启水印','开启水印后水印图片优先，如果没有图片则使用水印文字','6','0','8','开启=1,关闭=0','100','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('71','watermark_file','水印图片','水印图片在250px以内','1', NULL,'8', NULL,'99','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('72','watermark_t','水印位置','参考键盘九宫格1-9','2','9','8', NULL,'98','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('73','watermark_tm','水印透明度','透明度越大，越难看清楚水印','6','0','8','不显示=0,10%=10,20%=20,30%=30,40%=40,50%=50,60%=60,70%=70,80%=80,90%=90','97','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('74','money_exchange','钱包兑换率','站内钱包与RMB的兑换率，即1元=多少金币','2','1','5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('75','jifen_exchange','积分兑换率','站内积分与RMB的兑换率，即1元=多少积分','2','100','5', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('76','isopenjifen','积分支付','开启积分支付后，商品可以用积分支付','6','1','5','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('77','isopenqianbao','钱包支付','开启钱包支付后，商品可以用钱包支付','6','1','5','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('78','isopenweixin','微信支付','开启微信支付后，商品可以用微信支付','6','1','5','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('79','isopenzfb','支付宝支付','开启支付宝支付后，商品可以用支付宝支付','6','1','5','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('80','login_award','每次登录奖励','每天登录奖励积分数，最小为0，每天登录只奖励一次','2','1','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('81','login_award_open','登录奖励','开启登录奖励后，登录后就会获得积分奖励','6','1','7','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('82','release_award_open','发布奖励','开启后，发布内容会奖励积分','6','1','7','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('83','release_award','每次发布奖励','每次发布内容奖励积分数','2','1','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('84','release_max_award','每天发布最高奖励','每天奖励不超过积分上限，设置0则无上限','2','0','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('85','collect_award_open','收藏奖励','开启后，发布内容被收藏会奖励积分','6','1','7','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('86','collect_award','每次收藏奖励','每次发布内容被收藏奖励积分数','2','1','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('87','collect_max_award','每天收藏最高奖励','每天奖励不超过积分上限，设置0则无上限','2','1000','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('88','likes_award_open','点赞奖励','开启后，发布内容被点赞会奖励积分','6','1','7','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('89','likes_award','每次点赞奖励','每次发布内容被点赞奖励积分数','2','1','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('90','likes_max_award','每天点赞最高奖励','每天奖励不超过积分上限，设置0则无上限','2','1000','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('91','comment_award_open','评论奖励','开启后，发布内容被评论会奖励积分','6','1','7','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('92','comment_award','每次评论奖励','每次发布内容被评论奖励积分数','2','1','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('93','comment_max_award','每天评论最高奖励','每天奖励不超过积分上限，设置0则无上限','2','1000','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('94','follow_award_open','关注奖励','开启后，用户被粉丝关注会奖励积分','6','1','7','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('95','follow_award','每次关注奖励','每次被关注奖励积分数','2','1','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('96','follow_max_award','每天关注最高奖励','每天关注奖励不超过积分上限，设置0则无上限','2','1000','7', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('97','isopenemail','发送邮件','是否开启邮件发送','6','1','4','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('98','closeweb','关闭网站','关闭网站后，前台无法访问，后台可以进入','6','0','1','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('99','closetip','关站提示', NULL,'3','抱歉！该站点已经被管理员停止运行，请联系管理员了解详情！','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('100','admin_save_path','后台文件存储路径','默认static/upload/{yyyy}/{mm}/{dd}，存储路径相对于根目录，最后不能带斜杠[ / ]','2','static/upload/{yyyy}/{mm}/{dd}','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('101','home_save_path','前台文件存储路径','默认static/upload/{yyyy}/{mm}/{dd}，存储路径相对于根目录，最后不能带斜杠[ / ]','2','static/upload/{yyyy}/{mm}/{dd}','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('102','isajax','是否开启前台AJAX','开启后AJAX，前台可以通过栏目链接+ajax=1获取JSON数据','6','0','2', '开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('104','invite_award_open','是否开启邀请奖励','开启邀请后则会奖励','6','0','7', '开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('105','invite_type','邀请奖励类型', NULL,'6','jifen','7', '积分=jifen,金币=money','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('106','invite_award','邀请奖励数量', NULL,'0','0','0', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('107','web_phone','网站手机', NULL,'2','0','1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('108','web_weixin','站长微信', NULL,'1', NULL,'1', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('110','isregister','前台用户注册','关闭前台注册后，前台无法进入注册页面','6','1','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('111','onlyinvite','仅邀请码注册','开启后，必须通过邀请链接才能注册！','6','0','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('112','release_table','允许前台发布模块','防止数据泄露,填写允许发布模块标识,留空表示不允许发布,多个表可用|分割','2','article|product','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('113','search_words','前台搜索的字段','可以设置搜索表中的相关字段进行模糊查询,多个字段可用|分割','2','title','3', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('114','closehomevercode','前台验证码','关闭后，登录注册不需要验证码','6','0','2','关闭=1,开启=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('115','closeadminvercode','后台验证码','关闭后，后台管理员登录不需要验证码','6','0','2','关闭=1,开启=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('116','tag_table','TAG包含模型','在tag列表上查询的相关模型,多个模型标识可用|分割,如：article|product','2','article|product','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('118','isopendmf','支付宝当面付', NULL,'6','1','5','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('119','search_words_muti','前台多模块搜索的字段','多个模块直接必须都有相同的字段，否则会报错','3','title','3', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('120','search_table_muti','多模块允许搜索的表','防止数据泄露,填写允许搜索的表名,留空表示不允许搜索,多个表可用|分割,如：article|product','2','article|product','3', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('121','search_fields_muti','允许查询显示的字段','多模块搜索允许查询显示的字段','3','id,tid,litpic,title,tags,keywords,molds,htmlurl,description,addtime,userid,member_id,hits,ownurl,target','3', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('122','ueditor_user_config','前台编辑器设置','前台的编辑器功能菜单设置','3','&quot;undo&quot;, &quot;redo&quot;, &quot;|&quot;,&quot;paragraph&quot;,&quot;bold&quot;,&quot;forecolor&quot;,&quot;fontfamily&quot;,&quot;fontsize&quot;, &quot;italic&quot;, &quot;blockquote&quot;, &quot;insertparagraph&quot;, &quot;justifyleft&quot;, &quot;justifycenter&quot;, &quot;justifyright&quot;,&quot;justifyjustify&quot;,&quot;|&quot;,&quot;indent&quot;, &quot;insertorderedlist&quot;, &quot;insertunorderedlist&quot;,&quot;|&quot;, &quot;insertimage&quot;, &quot;inserttable&quot;, &quot;deletetable&quot;, &quot;insertparagraphbeforetable&quot;, &quot;insertrow&quot;, &quot;deleterow&quot;, &quot;insertcol&quot;, &quot;deletecol&quot;,&quot;mergecells&quot;, &quot;mergeright&quot;, &quot;mergedown&quot;, &quot;splittocells&quot;, &quot;splittorows&quot;, &quot;splittocols&quot;, &quot;|&quot;,&quot;drafts&quot;, &quot;|&quot;,&quot;fullscreen&quot;','2', NULL,'1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('123','article_config','内容配置', NULL,'3','{"seotitle":1,"litpic":1,"description":1,"tags":1,"filter":"title,keywords,body"}','0', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('124','product_config','商品配置', NULL,'3','{"seotitle":1,"litpic":1,"description":1,"tags":1,"filter":"title,keywords,body"}','0', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('125','isdebug','PHP调试','测试环境，开启调试，提示错误，实时更新模板。正式上线，请关闭调试，打开页面更快。','6','1','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('126','plugins_config','插件配置', NULL,'2','http://api.jizhicms.cn/plugins.php','0', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('127','template_config','插件配置', NULL,'2','http://api.jizhicms.cn/template.php','0', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('128','closesession','前台SESSION','关闭前台SESSION后，前台会员模块无法使用，但是可以减少session缓存文件。纯内容网站可以关闭，使用会员支付等必须开启','6','0','2','关闭=1,开启=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('129','messageyzm','留言验证码','开启后，前台留言需要填写验证码','6','1','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('130','homerelease','前台发布审核','开启后需要后台审核，关闭则不需要','6','1','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('131','hideclasspath','栏目隐藏.html','开启后栏目链接将没有.html后缀','6','0','2','开启=1,关闭=0','0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('132','classtypemaxlevel','栏目全局递归','默认开启，栏目超过20个，请关闭此选项，有一定程度提升访问速度！','6','0','2','开启=1,关闭=0','1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('133','hidetitleonliy','字段重复检测', '将【模块标识-检测字段】填写进去，用|进行分割，将会进行标题重复检测。如：article-title|product-title','2','article-title|product-title','2', NULL,'0','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('134','onlyuserupload','会员上传限制','开启后，仅会员才可以上传！受会员上传大小限制！','6','1','2','开启=1,关闭=0','1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('135','cachefilenum','缓存文件数','0表示不限制，最大不超过500','2','100','0',null,0,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('136','watermark_word','水印文字','只有没有水印图片的时候才生效','2','这个是水印文字','8',null,96,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('137','watermark_font','水印字体','默认simsun.ttf，存放在static/common','2','simsun.ttf','8',null,95,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('138','watermark_size','水印大小','默认24','2','24','8',null,94,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('139','watermark_h','水印行高','默认34','2','34','8',null,93,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('140','watermark_rgb','水印颜色','默认白色：#FFFFFF','2','#FFFFFF','8',null,92,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('141','watermark_x','水印微调X','相对水印位置再进行X轴微调，默认0','2','0','8',null,91,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('142','watermark_y','水印微调Y','相对水印位置再进行Y轴微调，默认0','2','10','8',null,90,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('143','text_waterlitpic','缩略图标题水印','文章缩略图进行水印文章标题，开启后生效','6','0','8','开启=1,关闭=0',89,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('144','text_litpic','默认缩略图','当文章没有缩略图的时候生效','1',null,'8',null,88,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('145','text_molds','支持模型','填写模型标识，如：article,product','2','article,product','8',null,87,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('146','text_num','每行文字数','默认10个字','2','10','8',null,86,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('147','text_size','文字大小','默认24','2','24','8',null,85,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('148','text_h','文字行高','默认34','2','34','8',null,84,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('149','text_rgb','文字颜色','默认白色：#FFFFFF','2','#FFFFFF','8',null,83,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('150','text_font','文字字体','默认simsun.ttf，存放在static/common','2','simsun.ttf','8',null,82,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('151','text_wz','水印位置','九宫格1-9，默认5中间','2','5','8',null,81,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('152','text_x','微调X','相对于水印位置再进行X轴微调，默认0','2','0','8',null,80,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('153','text_y','微调Y','相对于水印位置再进行Y轴微调，默认0','2','0','8',null,79,'1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('154','islocal','是否开启图片本地化','图片本地化可以将内容的外网图片保存到服务器','6','1','2','开启=1,关闭=0','1','1');
INSERT INTO `jz_sysconfig` (`id`,`field`,`title`,`tip`,`type`,`data`,`typeid`,`config`,`orders`,`sys`) VALUES ('155','openredis','是否开启Redis','开启Redis后可以使用token登录前台账户，但必须服务器安装了Redis，在config里面需要配置redis信息','6','0','2','开启=1,关闭=0','1','1');
-- ----------------------------
-- Records of jz_tags
-- ----------------------------
INSERT INTO `jz_tags` (`id`,`tid`,`orders`,`comment_num`,`molds`,`htmlurl`,`keywords`,`newname`,`num`,`isshow`,`target`,`number`,`member_id`,`ownurl`,`tags`,`addtime`) VALUES ('1','0','0','0','tags', NULL,'SEO', NULL,'-1','1','_blank','4','0', NULL, NULL,'0');
-- ----------------------------
-- Records of jz_task
-- ----------------------------
INSERT INTO `jz_task` (`id`,`tid`,`aid`,`userid`,`puserid`,`molds`,`type`,`body`,`url`,`isread`,`isshow`,`readtime`,`addtime`) VALUES ('1','8','2','1','1','article','reply',' @iPHfa6 干得漂亮！','http://www.19x.mm/znxw.html','1','1','0','1642932172');
INSERT INTO `jz_task` (`id`,`tid`,`aid`,`userid`,`puserid`,`molds`,`type`,`body`,`url`,`isread`,`isshow`,`readtime`,`addtime`) VALUES ('5','6','9','0','1','product','likes','PC+手机绿色医疗生物化工网站模板','http://www.19x.mm/free/9.html','0','1','0','1642946653');
INSERT INTO `jz_task` (`id`,`tid`,`aid`,`userid`,`puserid`,`molds`,`type`,`body`,`url`,`isread`,`isshow`,`readtime`,`addtime`) VALUES ('6','7','8','0','1','product','likes','手机端黄色五金机电网站模板','http://www.19x.mm/business/8.html','0','1','0','1642946655');
INSERT INTO `jz_task` (`id`,`tid`,`aid`,`userid`,`puserid`,`molds`,`type`,`body`,`url`,`isread`,`isshow`,`readtime`,`addtime`) VALUES ('11','6','10','0','1','product','collect','蓝色小程序鲜花礼物广告设计网站模板','http://www.19x.mm/free/10.html','0','1','0','1642947563');
INSERT INTO `jz_task` (`id`,`tid`,`aid`,`userid`,`puserid`,`molds`,`type`,`body`,`url`,`isread`,`isshow`,`readtime`,`addtime`) VALUES ('10','7','7','0','1','product','collect','响应式红色软件公司网站模板','http://www.19x.mm/business/7.html','0','1','0','1642946850');

*/