# 商机提取器 BOG

商机提取器，英文名 Business Opportunity Grabber，缩写 BOG，是一个计划基于 Go + Wails 构建的桌面应用。它用于从国家电网、南方电网等可配置网站入口中获取招标需求、采购公告、项目公告等商机信息，并提供筛选、去重、归档和导出能力。

## 1. 产品定位

BOG 面向需要持续关注电力行业招标采购机会的用户。

核心目标：

- 支持手动配置目标网站入口
- 抓取公告列表和公告详情
- 按关键词、地区、时间、公告类型筛选
- 自动去重并保存历史记录
- 支持导出 Excel、CSV 或 JSON
- 为后续定时监控和提醒预留能力

首个示例站点：

```text
https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe
```

该地址对应国家电网电子商务平台相关公告列表页面。

## 2. 应用名称

- 中文名：商机提取器
- 英文名：Business Opportunity Grabber
- 缩写：BOG

## 3. 目标用户

- 电力工程公司商务人员
- 招投标专员
- 售前和市场人员
- 关注国家电网、南方电网项目机会的企业用户

## 4. 核心功能大纲

### 4.1 站点配置

用户可以在应用内手动维护目标网站。

计划字段：

- 站点名称
- 站点类型：国家电网、南方电网、自定义
- 入口 URL
- 是否启用
- 抓取模式：HTTP 接口 / 浏览器渲染
- 默认关键词
- 默认地区
- 默认时间范围
- 访问间隔
- 重试次数

国家电网示例配置：

```json
{
  "name": "国家电网 - 招标采购公告",
  "siteType": "sgcc",
  "url": "https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe",
  "enabled": true,
  "renderMode": "browser"
}
```

### 4.2 抓取任务

用户可以手动启动抓取任务。

计划能力：

- 选择一个或多个站点
- 设置抓取时间范围
- 设置关键词
- 查看任务进度
- 查看成功、失败、跳过、重复数量
- 查看错误日志

后续可扩展：

- 定时抓取
- 后台静默运行
- 新公告提醒

### 4.3 公告列表

公告列表应适合快速扫描。

计划字段：

- 标题
- 来源站点
- 公告类型
- 发布时间
- 地区
- 招标编号
- 采购单位
- 截止时间
- 匹配关键词
- 原文链接
- 抓取时间

列表能力：

- 搜索标题
- 按站点筛选
- 按地区筛选
- 按公告类型筛选
- 按发布时间排序
- 只看新公告
- 只看已收藏

### 4.4 公告详情

详情页展示原始公告的关键字段和正文摘要。

计划内容：

- 标题
- 来源链接
- 发布时间
- 正文内容
- 附件列表
- 命中的关键词
- 用户备注
- 收藏状态

### 4.5 去重逻辑

建议优先使用组合规则：

- 来源站点
- 原文 URL
- 公告标题
- 发布时间
- 招标编号

如果存在公告编号，以公告编号作为强去重字段。

### 4.6 数据导出

计划支持：

- Excel `.xlsx`
- CSV
- JSON

导出字段应可配置，方便用户只导出标题、链接、发布时间、采购单位等核心字段。

## 5. 技术架构设计

### 5.1 总体架构

```text
Wails Desktop App
  Frontend UI
    - 站点配置
    - 抓取任务
    - 公告列表
    - 公告详情
    - 导出中心
  Go Backend
    - 配置管理
    - 抓取调度
    - 页面抓取
    - 数据解析
    - SQLite 存储
    - 文件导出
```

### 5.2 后端模块

建议模块：

- `config`：读取和保存站点配置
- `crawler`：执行网页抓取
- `parser`：解析公告列表和详情
- `storage`：SQLite 数据访问
- `scheduler`：任务调度和状态管理
- `export`：导出 Excel、CSV、JSON
- `logger`：运行日志

### 5.3 前端页面

建议页面：

- 首页仪表盘
- 站点管理
- 抓取任务
- 公告库
- 公告详情
- 关键词规则
- 导出记录
- 设置

## 6. 数据模型草案

### 6.1 SiteConfig

```text
id
name
site_type
base_url
enabled
render_mode
keywords
regions
date_range_days
min_interval_ms
max_retries
created_at
updated_at
```

### 6.2 Opportunity

```text
id
site_id
source_site
title
notice_type
publish_time
region
tender_no
buyer
deadline
source_url
content
matched_keywords
is_favorite
remark
content_hash
created_at
updated_at
```

### 6.3 CrawlTask

```text
id
site_id
status
started_at
finished_at
total_count
new_count
duplicate_count
failed_count
error_message
```

## 7. 国家电网页面适配思路

目标入口：

```text
https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe
```

初期建议使用浏览器渲染模式验证页面结构和数据加载方式。

验证重点：

- 页面是否需要登录
- 公告列表是否由接口加载
- 是否存在分页接口
- 详情页链接格式是否稳定
- 发布时间、标题、公告类型等字段是否可稳定提取

如果能识别出稳定接口，再增加 HTTP 接口模式以提升抓取速度。

## 8. 南方电网适配思路

南方电网入口先不固定到代码中，应通过站点管理手动录入。

适配策略：

- 先作为 `custom` 站点保存
- 验证列表页和详情页结构
- 提炼字段映射规则
- 稳定后升级为内置 `csg` 站点模板

## 9. 非目标范围

初期不做：

- 自动绕过验证码
- 绕过登录权限
- 高频爬取
- 分布式采集
- 大规模数据售卖接口

## 10. 开发里程碑

### Milestone 1：项目骨架

- 初始化 Wails 项目
- 建立基础页面
- 建立配置文件和 SQLite 连接

### Milestone 2：站点配置

- 新增、编辑、删除站点
- 保存国家电网示例 URL
- 支持启用和停用

### Milestone 3：国家电网抓取验证

- 打开配置 URL
- 获取公告列表
- 解析标题、时间、链接
- 保存到本地数据库

### Milestone 4：公告库

- 列表展示
- 搜索和筛选
- 详情查看
- 收藏和备注

### Milestone 5：导出和任务日志

- 导出 Excel/CSV
- 抓取任务状态
- 错误日志

### Milestone 6：南方电网适配

- 支持手动配置南方电网入口
- 增加字段映射
- 沉淀为内置模板

