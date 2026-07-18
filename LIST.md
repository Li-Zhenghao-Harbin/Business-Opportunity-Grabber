# 商机提取器 BOG

这是“商机提取器”桌面应用的 Wails 实现目录。

## 已实现功能

- 默认内置国家电网站点：`https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe`
- 支持新增、编辑、删除、启用、停用站点配置
- 支持手动选择一个或多个站点执行抓取
- 支持关键词、收藏、站点筛选公告库
- 支持公告收藏、详情查看、备注保存
- 支持抓取任务日志
- 本地数据持久化到用户配置目录下的 `Business Opportunity Grabber/bog-data.json`

## 开发运行

```powershell
wails dev
```

Wails 开发模式会启动桌面应用，并提供前端热更新。

## 生产构建

```powershell
wails build
```

Windows 构建产物：

```text
build/bin/business-opportunity-grabber.exe
```

## 当前抓取说明

当前版本实现的是 HTTP 静态页面抓取和通用公告链接解析。国家电网页面是前端单页应用，列表数据很可能由异步接口加载；如果静态 HTML 中没有公告列表，应用会保存一条“抓取提示”记录，提醒后续需要分析真实列表接口或增加浏览器渲染适配。

