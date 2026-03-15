# 🧚 Navi

> *"Hey! Listen!"* — 你的 AI Agent 浏览器调试精灵

Navi 是一个轻量级 CDP (Chrome DevTools Protocol) 客户端 CLI，让 AI Agent 能直接连接浏览器，自主完成前端调试——不再需要人工截图、粘贴 console、充当 Agent 的眼睛和手。

## 为什么叫 Navi？

塞尔达传说里的小精灵 Navi，总在你耳边喊 *"Hey! Listen!"*，帮你发现隐藏的线索。

这个工具做的事情一样：**替 Agent 盯着浏览器，把 console 报错、API 失败、页面状态这些"隐藏线索"主动喊出来。**

## 效果对比

**之前（人工中转）：**
```
人截图 → 发 Discord → Agent 用 image API 分析（可能限流）
→ Agent 猜问题 → 让人粘 console → 人粘贴 → Agent 再分析
→ 3-5 轮对话, 10-30 分钟
```

**之后（Navi）：**
```
Agent: navi console --errors + navi network --failed
→ 1 轮, < 10 秒，直接修代码
```

## 安装

```bash
# 从 release 下载（单二进制，无依赖）
curl -L https://github.com/user/navi/releases/latest/download/navi-$(uname -s | tr A-Z a-z)-$(uname -m) -o navi
chmod +x navi
sudo mv navi /usr/local/bin/
```

或从源码构建：

```bash
git clone https://github.com/user/navi.git
cd navi
go build -o navi .
```

## 快速开始

### 1. 启动 Chrome（开启调试端口）

```bash
# macOS (Chrome 145+ 必须指定 user-data-dir)
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 \
  --user-data-dir=/tmp/navi-chrome-profile

# Linux
google-chrome --remote-debugging-port=9222 --user-data-dir=/tmp/navi-chrome-profile
```

> ⚠️ Chrome 145+ 要求 `--remote-debugging-port` 必须搭配 `--user-data-dir`（非默认路径），否则端口参数被静默忽略。

### 2. 使用 Navi

```bash
# 查看所有打开的 tab
navi tabs

# 获取 console 错误
navi console --errors

# 查看失败的 API 请求（401、500 等）
navi network --failed

# 看某个接口的详细响应
navi network --filter "/api/v1/devices" --last --body

# 截图
navi screenshot

# 截取特定元素
navi screenshot --selector ".main-content"

# 执行 JS 检查页面状态
navi eval "document.querySelector('.ws-status').innerText"

# 查询 DOM 元素
navi query ".alarm-panel .status" --text

# 查看 localStorage
navi storage --local
```

## 命令一览

| 命令 | 功能 | 典型场景 |
|------|------|---------|
| `tabs` | 列出浏览器所有 tab | 选择调试目标 |
| `console` | 获取控制台日志 | `--errors` 只看报错 |
| `network` | 获取网络请求 | `--failed` 看 401/500 等失败请求 |
| `screenshot` | 页面截图 | `--selector` 截取特定元素 |
| `eval` | 执行 JavaScript | 检查运行时状态 |
| `fetch` | API 请求（自动认证） | 自动读 token，一行拿 API 数据 |
| `query` | 查询 DOM 元素 | 提取文本、属性、HTML |
| `perf` | 性能指标 | 页面加载耗时、DOM 节点数 |
| `storage` | 查看存储 | localStorage / sessionStorage / cookie |

## 全局参数

```bash
# 指定 CDP 连接地址（默认 ws://localhost:9222）
navi --url ws://192.168.1.100:9222 console --errors

# 按 URL 选择 tab
navi --tab-url "*acoplat*" network --failed

# JSON 输出（方便 Agent 解析）
navi console --errors --json

# 持续监听模式
navi console --follow
navi network --follow
```

## 连接方式

| 场景 | 配置 |
|------|------|
| 本地 Chrome（同一台机器） | 默认即可，`ws://localhost:9222` |
| 远程机器的 Chrome | SSH 隧道：`ssh -L 9222:localhost:9222 user@host`，然后本地连接 |

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `NAVI_CDP_URL` | CDP WebSocket 地址 | `ws://localhost:9222` |

## 技术栈

- **语言**: Go
- **CDP**: chromedp
- **CLI**: cobra
- **产物**: 单二进制 ~10MB，无 CGO，支持交叉编译

## 支持平台

| 平台 | 架构 |
|------|------|
| Linux | amd64, arm64 |
| macOS | arm64 (Apple Silicon) |

## 详细设计

见 [docs/design.md](docs/design.md)

## License

MIT
