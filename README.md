# 商机提取器

商机提取器是一个基于 Go + Wails + Vue 的桌面应用，用于抓取、保存和查看招标采购公告。

当前版本已内置国家电网电子商务平台的七类公告栏目和南方电网供应链统一服务平台“采购公告”，可通过手动或定时方式抓取公告数据。

## 已实现功能

- 公告库：查看已抓取公告，支持按标题、正文、来源搜索，以及按站点筛选。
- 公告详情：展示公告标题、来源、类型、发布时间、发布单位、项目状态、项目编号、截止时间和内容摘要。
- 打开原文：使用系统默认浏览器打开公告原文页面。
- 站点配置：新增、编辑、删除、启用或停用站点。
- 国家电网抓取：内置七个国网 ECP 公告栏目，支持分页、栏目水位和每日增量抓取。
- 公告归档：可配置本地归档目录，保存离线公告摘要，并尝试归档公开详情和附件；失败项可手动重新归档。
- 南方电网抓取：内置适配南方电网采购公告静态列表页。
- 手动抓取：点击“开始抓取”后抓取所有启用站点最近 7 天数据。
- 全自动模式：按 PDF 第 1-7 页顺序处理国网单一来源采购事前公示、年度采购计划预安排和资格预审公告，展示步骤进度并归档 Word 文档。
- 定时抓取：可开启自动抓取并设置间隔分钟，默认 60 分钟。
- 任务日志：记录每次抓取的状态、新增数量、重复/更新数量、失败数量和错误信息。
- 本地存储：数据保存到用户配置目录下的 JSON 文件。

## 国网 P0 规格

国网的栏目、增量、归档和验收场景采用 SDD 文档维护：

- [需求规格](docs/sdd/001-sgcc-p0/requirements.md)
- [设计规格](docs/sdd/001-sgcc-p0/design.md)
- [实施任务](docs/sdd/001-sgcc-p0/tasks.md)

全自动模式的规格与验收场景见 [002-sgcc-auto-pages-1-7](docs/sdd/002-sgcc-auto-pages-1-7/requirements.md)。

## 当前内置站点

```text
https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe
https://www.bidding.csg.cn/zbcg/index.jhtml
```

该站点使用国家电网 ECP 的公告列表接口：

```text
https://ecp.sgcc.com.cn/ecp2.0/ecpwcmcore/index/noteList
```

当前抓取菜单为“招标公告及投标邀请书”，菜单 ID：

```text
2018032700291334
```

南方电网站点使用静态公告列表页：

```text
https://www.bidding.csg.cn/zbcg/index.jhtml
```

## 抓取模式

应用内仍保留抓取模式字段：

- HTTP 静态抓取：当前可用。国家电网站点会走已适配的真实接口，南方电网站点会走专用列表解析，其他自定义站点会尝试直接解析静态 HTML。
- 浏览器渲染：界面中标记为“未启用”，当前版本尚未实现。

## 数据保存位置

当前数据文件：

```text
%APPDATA%\商机提取器\opportunity-data.json
```

## 开发运行

```powershell
wails dev
```

## 前端构建

```powershell
cd frontend
npm run build
```

## Go 测试

```powershell
go test ./...
```

## 生产构建

```powershell
wails build
```

Windows 构建产物：

```text
build/bin/business-opportunity-grabber.exe
```
