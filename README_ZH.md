# gscp

[English README](./README.md)

`gscp` 是一个使用 Go 编写的 CLI 工具，用来把本地文件或目录上传到远程服务器，然后通过 SSH 执行远程命令。

当前支持：

- 本地管理服务器配置
- 在当前目录初始化默认 `.genv`
- 使用 Bubble Tea 交互式选择环境
- 上传过程实时进度展示
- 顺序执行远程命令
- 通过 `sudo -S` 支持 `sudo`
- 通过 `gscp run -g <group_name>` 执行环境组
- 通过 `gscp add -r <json_url>` 导入远程服务器配置
- 通过 `gscp serve` 启动本地 Web UI，管理服务器、工作区和 `.genv` 文件
- 执行 `init` 和 `run` 时自动记录工作区路径
- 扫描文件系统，发现可配置根目录下的所有 `.genv` 文件
- 可配置扫描设置（跳过目录、扫描根目录）

## 构建

```bash
go build -o gscp .
```

开发时也可以直接运行：

```bash
go run . <command>
```

## 命令

```bash
gscp add <alias> <host[:port]> <username> <password>
gscp add -r <json_url>
gscp init
gscp ls
gscp rm <alias>
gscp run
gscp run <env_key>
gscp run -d
gscp run -g <group_name>
gscp serve [addr]
```

## 服务器管理

手动添加单个服务器：

```bash
gscp add prod 192.168.1.10 root mypassword
gscp add prod 192.168.1.10:2222 root mypassword
```

如果没有填写端口，SSH 默认使用 `22`。

从远程 JSON 地址导入服务器配置：

```bash
gscp add -r https://example.com/servers.json
```

远程 JSON 的结构需要和本地 `servers.json` 一致：

```json
{
  "servers": {
    "prod": {
      "alias": "prod",
      "host": "192.168.1.10:2222",
      "username": "root",
      "password": "mypassword"
    },
    "staging": {
      "alias": "staging",
      "host": "192.168.1.20",
      "username": "deploy",
      "password": "secret"
    }
  }
}
```

`add -r` 的融合规则：

- 本地没有的别名会直接追加
- 如果别名已存在，远程配置会覆盖本地配置
- 只接受 `http://` 和 `https://` 地址

查看所有服务器：

```bash
gscp ls
```

删除服务器：

```bash
gscp rm prod
```

服务器配置会保存在当前用户配置目录下的 `gscp/servers.json`。

## 初始化 `.genv`

```bash
gscp init
```

这会在当前目录生成一个默认 `.genv`。

如果你之前已经通过 `gscp add` 保存过服务器，`init` 会尽量自动把第一个服务器别名填到 `active_alias`。

如果当前目录已经存在 `.genv`，`init` 不会覆盖它。

## `.genv` 格式

示例：

```json
{
  "groups": {
    "default": ["dev"],
    "prod-all": ["web", "worker"]
  },
  "dev": {
    "active_alias": "dev-server",
    "is_default": true,
    "local_path": "./dist",
    "to_path": "/var/www/dev",
    "commands": [
      "cd /var/www/dev",
      "sudo systemctl restart dev-app"
    ]
  },
  "web": {
    "active_alias": "web-server",
    "is_default": false,
    "local_path": "./dist",
    "to_path": "/var/www/web",
    "commands": [
      "cd /var/www/web",
      "sudo systemctl restart web-app"
    ]
  },
  "worker": {
    "active_alias": "worker-server",
    "is_default": false,
    "local_path": "./dist",
    "to_path": "/var/www/worker",
    "commands": [
      "cd /var/www/worker",
      "sudo systemctl restart worker-app"
    ]
  }
}
```

字段说明：

- `groups`：可选，定义环境组，供 `gscp run -g <group_name>` 使用
- `active_alias`：服务器别名，对应你之前通过 `gscp add` 保存的服务器
- `is_default`：默认环境，供 `gscp run -d` 使用
- `local_path`：本地要上传的文件或目录
- `to_path`：远程目标目录
- `commands`：上传完成后要执行的命令列表

## 运行部署

交互式选择环境：

```bash
gscp run
```

直接执行某个环境：

```bash
gscp run pro
```

直接执行默认环境：

```bash
gscp run -d
```

执行一个环境组：

```bash
gscp run -g prod-all
```

组内环境会按照 `.genv` 里定义的顺序依次执行。

## `run` 的执行流程

`gscp` 会按下面的顺序执行：

1. 读取当前目录下的 `.genv`
2. 解析目标环境或环境组
3. 根据 `active_alias` 找到本地保存的服务器配置
4. 通过 SSH 连接远程服务器
5. 使用 Bubble Tea 界面展示上传进度
6. 上传本地文件或目录
7. 按顺序执行 `commands`
8. 在 TUI 日志区显示命令输出

## `sudo` 支持

如果某条命令以 `sudo ` 开头，`gscp` 会自动改写为使用已保存的服务器密码配合 `sudo -S` 执行。

同时它会为远程命令申请一个 TTY，所以对 “必须在 TTY 中执行 sudo” 的服务器也更友好。

## 工作区记录

每次执行 `gscp init` 或 `gscp run` 时，`gscp` 会自动把当前目录追加到全局配置的 `workspaces` 列表中（去重）。这样 Web UI 就能发现所有曾经使用过 `gscp` 的项目目录，方便统一管理。

## Web UI（`gscp serve`）

启动本地 Web 服务器，通过浏览器管理服务器配置、工作区和 `.genv` 文件：

```bash
gscp serve          # 监听 :8080
gscp serve :9090    # 自定义端口
```

在浏览器中打开 `http://localhost:8080`。

### REST API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/servers` | 列出所有服务器配置 |
| `POST` | `/api/servers` | 新增或更新服务器配置 |
| `PUT` | `/api/servers/{alias}` | 按别名更新服务器配置 |
| `DELETE` | `/api/servers/{alias}` | 按别名删除服务器配置 |
| `GET` | `/api/workspaces` | 列出所有已记录的工作区路径 |
| `POST` | `/api/workspaces/add` | 手动添加工作区路径 |
| `DELETE` | `/api/workspaces` | 移除工作区路径（请求体：`{"path":"..."}`） |
| `POST` | `/api/genv/read` | 读取并解析 `.genv` 文件（请求体：`{"path":"..."}`） |
| `POST` | `/api/genv/write` | 将原始 JSON 写入 `.genv` 文件（请求体：`{"path":"...","raw":"..."}`） |
| `GET` | `/api/scan` | 扫描 `.genv` 文件（Server-Sent Events 流） |
| `GET` | `/api/settings` | 获取当前扫描设置 |
| `PUT` | `/api/settings` | 更新扫描设置 |

### 扫描 API（SSE）

`GET /api/scan` 会以 Server-Sent Events 的形式推送三种事件：

- `scanning` — `{"dir": "<当前正在进入的目录>"}`
- `found` — `{"path": "<包含 .genv 的目录>"}`
- `done` — `{"count": N}`

### 扫描设置

扫描设置控制哪些目录会被跳过，以及从哪些根目录开始扫描：

```json
{
  "skip_dirs": [".git", "node_modules", "vendor", "dist", "build"],
  "scan_roots": ["/home/user/projects"]
}
```

- `skip_dirs`：扫描时跳过的目录名。默认使用内置列表（`.git`、`node_modules`、`vendor`、`dist`、`build` 等）。
- `scan_roots`：扫描的根目录列表。为空时默认使用用户主目录。

设置会持久化保存在全局 `servers.json` 配置文件中。

## 注意事项

- 服务器密码目前仍然是明文保存在本地配置里
- SSH host key 校验当前使用的是 `InsecureIgnoreHostKey`
- TUI 日志区目前只保留最近一部分日志

## 示例流程

```bash
gscp add demo 10.0.0.8:2222 root secret123
gscp init
gscp run
```

或者先导入远程服务器配置：

```bash
gscp add -r https://example.com/servers.json
gscp run -g default
```
