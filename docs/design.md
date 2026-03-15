# navi 设计文档

## 1. 项目概述

### 1.1 背景

当前调试 ACOPlat Cloud 前端存在以下痛点：

- Agent 无法直接看到浏览器页面，需要人工截图发送
- 截图通过 image API 分析，经常遭遇限流
- 前端 console 报错需要人手动复制粘贴
- Agent 无法主动检查页面状态、网络请求、DOM 元素
- 调试循环极度低效——**人充当了 Agent 的眼睛和手**

### 1.2 目标

构建一个 **Go 实现的轻量 CDP (Chrome DevTools Protocol) 客户端 CLI**，让 Agent 能够直接连接浏览器，自主完成前端调试工作，消除对人工中转的依赖。

### 1.3 非目标

- 不做通用浏览器自动化 / 爬虫框架
- 不替代 Playwright / Puppeteer 等测试框架
- 不实现页面交互操作（点击、填表等），聚焦 **只读式调试**

---

## 2. 架构设计

### 2.1 总体架构

```
navi (Go CLI, ~10MB 单二进制)
    │
    ├── 连接层
    │   └── CDP over WebSocket → 目标浏览器
    │       - 开发机: chrome --remote-debugging-port=9222
    │       - Mac mini: 本地 Chrome（带界面，开启调试端口）
    │       - 远程: SSH 隧道转发
    │
    ├── 命令层
    │   └── 预置命令集 (cobra subcommands)
    │       console / network / screenshot / eval / query / perf / storage
    │
    └── 输出层
        ├── 人类可读格式 (默认)
        ├── --json 机器可读格式
        └── 文件输出 (screenshot → PNG)
```

### 2.2 连接管理

| 场景 | 连接方式 |
|------|---------|
| Mac mini 本地 Chrome | 直连 `ws://localhost:9222`（Chrome 带界面运行） |
| 远程开发机 | SSH 隧道 `ssh -L 9222:localhost:9222 user@host` 后直连 |

连接参数通过以下方式配置（优先级从高到低）：

1. 命令行参数 `--remote-debugging-url ws://host:port`
2. 环境变量 `DEVTOOL_CDP_URL`
3. 默认值 `ws://localhost:9222`

### 2.3 Tab 选择策略

浏览器通常有多个 tab，工具需要自动选择正确的 target：

1. `--tab-url` 参数：按 URL 模式匹配（支持 glob）
2. `--tab-title` 参数：按标题匹配
3. `--tab-index` 参数：按索引指定
4. 默认：选择第一个 `type=page` 的 tab
5. `tabs` 子命令：列出所有可用 tab

---

## 3. 命令设计

### 3.1 命令总览

| 命令 | 功能 | 解决什么痛点 |
|------|------|-------------|
| `console` | 获取控制台日志 | 不用人手动粘贴 console 输出 |
| `network` | 获取网络请求信息 | 不用人开 DevTools 截图 API 调用 |
| `screenshot` | 页面截图 | Agent 主动截图，不用找人要 |
| `eval` | 执行 JavaScript | 直接检查页面运行时状态 |
| `query` | 查询 DOM 元素 | 提取元素文本/属性 |
| `perf` | 性能指标 | 获取页面加载性能数据 |
| `storage` | 查看存储 | 检查 localStorage/cookie 等 |
| `tabs` | 列出浏览器 tab | 选择调试目标 |

### 3.2 console 命令

获取浏览器控制台输出。

```bash
# 获取所有 console 输出（最近 50 条）
navi console

# 只看错误
navi console --errors

# 只看警告和错误
navi console --level error,warn

# 持续监听模式（实时输出新日志）
navi console --follow

# 限制条数
navi console --limit 20

# JSON 输出
navi console --errors --json
```

**输出字段：**
- `timestamp` — 时间戳
- `level` — log / warn / error / info / debug
- `text` — 日志内容
- `source` — 来源文件和行号
- `stackTrace` — 错误堆栈（仅 error 级别）

**实现方式：**
- 启用 CDP `Runtime.enable`
- 监听 `Runtime.consoleAPICalled` 事件
- 监听 `Runtime.exceptionThrown` 事件

### 3.3 network 命令

获取网络请求和响应信息。

```bash
# 获取所有请求（最近 50 条）
navi network

# 只看失败请求（非 2xx）
navi network --failed

# 按 URL 过滤
navi network --filter "/api/v1/devices"

# 看最近一条匹配的请求详情（含响应体）
navi network --filter "/api/v1/devices" --last --body

# 只看特定 HTTP 方法
navi network --method POST

# 持续监听模式
navi network --follow

# JSON 输出
navi network --failed --json
```

**输出字段：**
- `url` — 请求 URL
- `method` — HTTP 方法
- `status` — 响应状态码
- `statusText` — 状态文本
- `duration` — 请求耗时 (ms)
- `requestHeaders` — 请求头（`--headers` 时显示）
- `responseHeaders` — 响应头（`--headers` 时显示）
- `responseBody` — 响应体（`--body` 时获取）
- `error` — 请求错误信息（如果失败）
- `type` — 资源类型 (XHR / Fetch / Document / Script 等)

**实现方式：**
- 启用 CDP `Network.enable`
- 监听 `Network.requestWillBeSent` — 记录请求
- 监听 `Network.responseReceived` — 记录响应状态码、headers、timing
- 按需调用 `Network.getResponseBody` — 获取响应体内容

**典型调试场景：**
```bash
# 场景1: 页面加载后 API 全部 401
navi network --failed
# 输出: GET /api/v1/devices 401 | GET /api/v1/alarms 401 | ...
# → Agent 判断 token 过期

# 场景2: 某个接口 500
navi network --filter "/api/v1/config" --last --body
# 输出: POST /api/v1/config 500 {"error":"invalid field: threshold"}
# → Agent 直接看到后端报错原因
```

### 3.4 screenshot 命令

页面截图，保存为 PNG。

```bash
# 全页面截图
navi screenshot

# 指定输出路径
navi screenshot -o /tmp/page.png

# 截取特定元素
navi screenshot --selector ".main-content"

# 全页面（含滚动区域）
navi screenshot --fullpage

# 指定视口大小
navi screenshot --viewport 1920x1080

# 截取后自动用 image tool 描述
navi screenshot --describe
```

**实现方式：**
- `Page.captureScreenshot` — 截取可视区域或全页面
- `DOM.getBoxModel` — 获取元素位置用于裁剪
- 输出路径默认 `/tmp/devtool-screenshot-{timestamp}.png`

### 3.5 eval 命令

在页面上下文中执行 JavaScript 表达式。

```bash
# 执行简单表达式
navi eval "document.title"

# 检查页面状态
navi eval "document.querySelector('.ws-status').innerText"

# 获取 WebSocket 连接状态
navi eval "window.__wsConnection?.readyState"

# 执行多行脚本（从 stdin）
echo 'JSON.stringify(window.__appState)' | navi eval -

# JSON 输出
navi eval --json "document.querySelectorAll('.alarm-item').length"
```

**实现方式：**
- `Runtime.evaluate` — 执行表达式并返回结果
- 自动序列化返回值（对象自动 JSON.stringify）

### 3.6 query 命令

查询 DOM 元素信息。

```bash
# 获取元素文本
navi query ".alarm-panel .status" --text

# 获取元素属性
navi query "#main-frame" --attr src

# 获取元素数量
navi query ".device-card" --count

# 获取元素 HTML
navi query ".error-message" --html

# 多个元素的文本列表
navi query ".nav-item" --text --all
```

**实现方式：**
- `Runtime.evaluate` 配合 `document.querySelector` / `querySelectorAll`
- 返回文本、属性、HTML 内容

### 3.7 perf 命令

获取页面性能指标。

```bash
# 获取页面加载性能
navi perf

# JSON 输出
navi perf --json
```

**输出字段：**
- `domContentLoaded` — DOMContentLoaded 耗时
- `loadComplete` — Load 事件耗时
- `firstPaint` — 首次绘制时间
- `firstContentfulPaint` — 首次内容绘制时间
- `domNodes` — DOM 节点数量
- `jsHeapSize` — JS 堆大小
- `layoutCount` — 布局次数

**实现方式：**
- `Performance.getMetrics` — 获取性能指标
- `Runtime.evaluate` 读取 `window.performance.timing`

### 3.8 storage 命令

获取浏览器存储内容。

```bash
# 查看 localStorage
navi storage --local

# 查看 sessionStorage
navi storage --session

# 查看 cookie
navi storage --cookie

# 按 key 过滤
navi storage --local --filter "token"

# JSON 输出
navi storage --local --json
```

**实现方式：**
- localStorage/sessionStorage：`Runtime.evaluate` 执行 JS 遍历
- cookie：`Network.getCookies`

### 3.9 tabs 命令

列出浏览器所有 tab。

```bash
# 列出所有 tab
navi tabs

# JSON 输出
navi tabs --json
```

**输出字段：**
- `index` — tab 索引
- `title` — 页面标题
- `url` — 页面 URL
- `type` — 类型 (page / background_page / service_worker)

---

## 4. 全局参数

| 参数 | 短参数 | 环境变量 | 默认值 | 说明 |
|------|--------|---------|--------|------|
| `--remote-debugging-url` | `-u` | `DEVTOOL_CDP_URL` | `ws://localhost:9222` | CDP WebSocket 地址 |
| `--tab-url` | | | | 按 URL 模式选择 tab |
| `--tab-title` | | | | 按标题选择 tab |
| `--tab-index` | `-t` | | `0` | 按索引选择 tab |
| `--json` | `-j` | | `false` | JSON 格式输出 |
| `--timeout` | | | `10s` | 命令超时时间 |
| `--verbose` | `-v` | | `false` | 详细日志 |

---

## 5. 技术选型

| 组件 | 选型 | 理由 |
|------|------|------|
| 语言 | Go 1.22+ | 单二进制、交叉编译、性能好 |
| CDP 库 | chromedp | Go 生态最成熟的 CDP 库 |
| CLI 框架 | cobra | Go CLI 标准选择 |
| JSON 解析 | encoding/json (标准库) | 无需额外依赖 |

### 5.1 依赖清单（预计）

```
github.com/chromedp/chromedp     — CDP 协议操作
github.com/chromedp/cdproto      — CDP 协议类型定义
github.com/spf13/cobra           — CLI 框架
```

极简依赖，无 CGO，交叉编译无障碍。

### 5.2 构建产物

| 平台 | 文件名 | 预计大小 |
|------|--------|---------|
| linux/amd64 | `devtool-linux-amd64` | ~10MB |
| linux/arm64 | `devtool-linux-arm64` | ~10MB |
| darwin/arm64 | `devtool-darwin-arm64` | ~10MB |

---

## 6. 部署方案

### 6.1 方案 A：连接开发机浏览器

```
开发机 (Windows/Mac)
├── Chrome --remote-debugging-port=9222
└── ACOPlat Cloud 页面已打开

      ↕ SSH 隧道

Mac mini (Agent 运行环境)
└── navi --remote-debugging-url ws://localhost:9222
```

**步骤：**
1. 开发机启动 Chrome：`chrome --remote-debugging-port=9222`
2. 建立 SSH 隧道：`ssh -L 9222:localhost:9222 开发机`
3. Agent 执行：`navi console --errors`

**优点：** 调试真实开发环境，所见即所得
**缺点：** 依赖隧道稳定性，开发机需保持运行

### 6.2 方案 B：Mac mini 本地 Chrome（推荐）

```
Mac mini
├── Chrome（带界面）--remote-debugging-port=9222
│   └── 用户已打开 ACOPlat Cloud 页面并登录
└── navi CLI（Agent 直接调用）
```

**步骤：**
1. Mac mini 上 Chrome 启动时加 `--remote-debugging-port=9222` 参数
2. 用户正常使用 Chrome 浏览 ACOPlat 页面（已登录）
3. Agent 直接执行 navi 命令，连接同一个 Chrome 实例

**优点：** 全自包含，无外部依赖；共享用户已有的登录态和页面状态；用户和 Agent 看到的是同一个页面
**缺点：** 需要 Chrome 以调试端口启动

### 6.3 推荐

**优先采用方案 B**（Mac mini 本地 Chrome），Agent 和用户共享同一个浏览器实例，辅以方案 A（按需连接远程开发机）。

---

## 7. 调试效率对比

### 现有流程（每个前端问题）

```
1. Agent 让人截图          → 等人响应 (1-5 min)
2. 人截图发 Discord         → 上传 (30s)
3. Agent 用 image API 分析  → 可能限流等待 (0-3 min)
4. Agent 让人粘 console     → 等人响应 (1-5 min)
5. 人粘贴 console 输出      → 发送 (30s)
6. Agent 分析，可能需要更多信息 → 重复 1-5
总计: 3-5 轮对话, 10-30 分钟
```

### 使用 navi 后

```
1. Agent 执行 navi console --errors    → 1s
2. Agent 执行 navi network --failed    → 1s
3. Agent 执行 navi screenshot          → 2s
4. 信息齐全，直接修代码
总计: 1 轮, < 10 秒
```

---

## 8. 开发计划

### Phase 1：核心功能（MVP）
- [ ] 项目骨架（Go module、cobra setup）
- [ ] CDP 连接管理（连接、tab 选择、断线重连）
- [ ] `tabs` 命令
- [ ] `console` 命令（含 `--follow`）
- [ ] `network` 命令（含 `--failed`、`--filter`、`--body`）
- [ ] `screenshot` 命令
- [ ] `eval` 命令
- [ ] 全局参数和 JSON 输出

### Phase 2：完整功能
- [ ] `query` 命令
- [ ] `perf` 命令
- [ ] `storage` 命令
- [ ] `console --follow` 和 `network --follow` 持续监听
- [ ] Tab URL/title 匹配

### Phase 3：体验优化
- [ ] 自动检测 Chrome 调试端口
- [ ] 连接状态健康检查
- [ ] 错误信息友好化
- [ ] Makefile / goreleaser 构建
- [ ] README 和使用文档

---

## 9. 灵感来源

项目灵感来自 @yan5xu 的 bb-browser 项目（Chrome 插件 + CDP 操控真实浏览器的思路），但 navi **不做通用浏览器自动化**，聚焦于 **Agent 辅助前端调试** 这一垂直场景。
