# 商机提取器 BOG 项目搭建说明

BOG 全称 Business Opportunity Grabber，中文名“商机提取器”。本项目计划使用 Go + Wails 构建桌面应用，用于从可配置的网站入口中获取招标、采购、公告等商机信息，例如：

- 国家电网电子商务平台：`https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe`
- 南方电网相关招标采购平台：后续由用户在应用内手动配置

本文档只描述项目构建和初始化方式，不包含业务代码实现。

## 1. 技术选型

### 桌面框架

- Wails v2
- Go 作为后端主进程
- WebView 承载前端界面

### 后端

- Go 1.22 或更高版本
- 负责配置管理、任务调度、数据抓取、解析、去重、存储和导出

### 前端

建议从以下方案中选择一种：

- Vue 3 + TypeScript + Vite
- React + TypeScript + Vite

如果没有特别偏好，建议使用 Vue 3 + TypeScript。理由是配置表单、任务列表、公告详情、筛选器等管理型界面实现成本较低。

### 本地数据

初期建议使用：

- SQLite：保存网站配置、抓取任务、公告记录、关键词规则、运行日志
- JSON/YAML：保存少量可编辑配置，例如默认站点模板

## 2. 环境要求

### Windows

需要安装：

- Go 1.22+
- Node.js 20+
- npm / pnpm
- Git
- Wails CLI
- WebView2 Runtime

安装 Wails CLI：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

检查环境：

```powershell
wails doctor
```

### macOS

需要安装：

- Go 1.22+
- Node.js 20+
- Xcode Command Line Tools
- Wails CLI

检查环境：

```bash
wails doctor
```

### Linux

需要安装：

- Go 1.22+
- Node.js 20+
- GTK/WebKit 相关依赖
- Wails CLI

不同发行版依赖不同，建议以 `wails doctor` 输出为准补齐。

## 3. 初始化项目

项目名称建议：

- 应用名：`商机提取器`
- 英文名：`Business Opportunity Grabber`
- 缩写：`BOG`
- Go module：`business-opportunity-grabber`

推荐初始化命令：

```powershell
wails init -n business-opportunity-grabber -t vue-ts
```

如果选择 React：

```powershell
wails init -n business-opportunity-grabber -t react-ts
```

初始化后建议保留如下目录结构：

```text
business-opportunity-grabber/
  app.go
  main.go
  go.mod
  wails.json
  frontend/
    package.json
    src/
  internal/
    config/
    crawler/
    parser/
    storage/
    scheduler/
    export/
  docs/
  SETUP.md
  README.md
```

说明：

- `internal/config`：站点路径、字段映射、关键词、抓取策略配置
- `internal/crawler`：网页请求、浏览器渲染、分页抓取、限速重试
- `internal/parser`：公告列表和详情页解析
- `internal/storage`：SQLite 数据访问
- `internal/scheduler`：手动抓取、定时抓取、任务状态
- `internal/export`：Excel、CSV、JSON 导出

## 4. 开发运行

进入项目目录后运行：

```powershell
wails dev
```

Wails 会启动 Go 后端和前端开发服务。

## 5. 打包应用

开发完成后执行：

```powershell
wails build
```

Windows 下产物通常位于：

```text
build/bin/
```

## 6. 站点配置设计

BOG 的核心能力之一是“手动配置指定网站路径”。不应把国家电网或南方电网的地址硬编码到业务逻辑中。

建议每个站点配置包含：

```json
{
  "id": "sgcc-list-spe",
  "name": "国家电网 - 招标采购公告",
  "enabled": true,
  "baseUrl": "https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe",
  "siteType": "sgcc",
  "renderMode": "browser",
  "listPage": {
    "url": "https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe",
    "pagination": "auto"
  },
  "filters": {
    "keywords": [],
    "regions": [],
    "dateRangeDays": 7
  },
  "rateLimit": {
    "minIntervalMs": 1500,
    "maxRetries": 3
  }
}
```

### 配置项说明

- `name`：用户可见的站点名称
- `baseUrl`：网站入口地址
- `siteType`：站点类型，例如 `sgcc`、`csg`、`custom`
- `renderMode`：抓取方式，建议支持 `http` 和 `browser`
- `keywords`：关注关键词，例如“变电站”、“输电线路”、“信息化”
- `regions`：区域筛选
- `dateRangeDays`：默认抓取最近多少天
- `rateLimit`：访问间隔和重试策略

## 7. 抓取方式建议

国家电网示例地址包含 hash 路由：

```text
https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe
```

这类页面通常是前端单页应用，列表数据可能由后台接口异步加载。因此建议优先支持两种模式：

### HTTP 接口模式

适合能分析出公告列表接口的站点。

优点：

- 速度快
- 资源占用低
- 解析稳定

缺点：

- 接口参数可能变化
- 需要单独适配每个站点

### 浏览器渲染模式

适合 SPA、动态加载、复杂筛选页面。

优点：

- 更接近真实用户访问
- 适合先快速验证

缺点：

- 速度较慢
- 对页面结构变化更敏感

Go 侧可评估使用：

- `chromedp`
- `rod`
- Wails 前端 WebView 配合后端任务

## 8. 合规和稳定性原则

BOG 应作为辅助检索工具，不应绕过网站安全措施。

建议：

- 尊重目标网站的访问频率
- 支持手动触发抓取，不默认高频轮询
- 保存抓取日志，便于追踪错误
- 不处理验证码绕过
- 不伪造登录或绕过权限
- 对来源 URL、抓取时间、公告发布时间完整留痕

## 9. 后续实施顺序

建议按以下顺序实现：

1. 初始化 Wails 项目
2. 完成站点配置 CRUD
3. 建立 SQLite 数据模型
4. 实现国家电网页面验证型抓取
5. 实现公告列表展示和关键词筛选
6. 实现公告详情入库和去重
7. 增加南方电网站点适配
8. 增加导出 Excel/CSV
9. 增加定时任务和运行日志

