# gscp

[English README](./README.md)

`gscp` 是一个使用 Go 编写的 CLI 工具，用于把本地文件上传到远程服务器，并在上传后执行远程命令。

它支持：

- 本地管理服务器配置
- 在当前目录初始化默认 `.genv`
- 使用 Bubble Tea 交互选择环境
- 展示上传进度
- 通过 SSH 执行远程命令
- 对 `sudo` 命令提供 `sudo -S` 支持

## 构建

```bash
go build -o gscp .
```

开发时也可以直接运行：

```bash
go run . <command>
```

## 命令说明

### 添加服务器

```bash
gscp add <alias> <host> <username> <password>
```

示例：

```bash
gscp add prod 192.168.1.10 root mypassword
```

这会把服务器配置保存到本地。

### 查看服务器列表

```bash
gscp ls
```

### 删除服务器

```bash
gscp rm <alias>
```

### 初始化默认 `.genv`

```bash
gscp init
```

这个命令会在当前目录生成一个 `.genv` 文件。

如果你之前已经通过 `add` 保存过服务器配置，`init` 会尝试自动把第一个服务器别名填入 `active_alias`。

如果当前目录已经存在 `.genv`，`init` 不会覆盖它。

## `.genv` 格式

示例：

```json
{
  "dev": {
    "active_alias": "dev-server",
    "is_default": true,
    "local_path": "./dist",
    "to_path": "/var/www/dev",
    "commands": [
      "cd /var/www/dev",
      "sudo systemctl restart app"
    ]
  },
  "pro": {
    "active_alias": "prod-server",
    "is_default": false,
    "local_path": "./dist",
    "to_path": "/var/www/prod",
    "commands": [
      "cd /var/www/prod",
      "sudo systemctl restart app"
    ]
  }
}
```

字段说明：

- `active_alias`：通过 `gscp add` 保存过的服务器别名
- `is_default`：`gscp run -d` 使用的默认环境
- `local_path`：本地文件或目录
- `to_path`：远程目标目录
- `commands`：上传完成后执行的远程命令

## 执行部署

### 交互选择环境

```bash
gscp run
```

这会打开一个 Bubble Tea 界面，让你选择环境。

### 直接执行指定环境

```bash
gscp run <env_key>
```

示例：

```bash
gscp run pro
```

### 直接执行默认环境

```bash
gscp run -d
```

这会执行 `.genv` 中 `is_default=true` 的环境。

规则：

- 如果没有任何环境设置 `is_default=true`，那么 `gscp run -d` 会报错
- 如果有多个环境设置了 `is_default=true`，那么 `gscp run -d` 也会报错

## `run` 时会做什么

`gscp` 在执行时会：

1. 读取当前目录下的 `.genv`
2. 选择目标环境
3. 根据 `active_alias` 找到对应服务器配置
4. 通过 SSH 建立连接
5. 上传本地文件或目录，并显示进度
6. 按顺序执行远程命令

远程命令的输出会显示在 TUI 的日志区域中。

## `sudo` 支持

如果某条命令以 `sudo ` 开头，`gscp` 会自动尝试使用保存的服务器密码配合 `sudo -S` 执行。

示例：

```json
{
  "dev": {
    "active_alias": "dev-server",
    "is_default": true,
    "local_path": "./dist",
    "to_path": "/var/www/app",
    "commands": [
      "sudo systemctl restart app"
    ]
  }
}
```

说明：

- `gscp` 会为远程命令申请一个 TTY，这样可以兼容要求 TTY 的 sudo 配置
- 如果服务器上 sudo 策略比较特殊，仍然可能需要单独调整

## 注意事项

- 服务器密码当前以明文形式保存在本地配置文件中
- SSH host key 校验当前使用的是 `InsecureIgnoreHostKey`
- 命令输出会显示在 Bubble Tea 的日志面板中
- 日志面板当前只保留最近一部分内容

## 本地配置位置

服务器配置会保存在当前用户配置目录下的：

```text
gscp/servers.json
```

## 示例流程

```bash
gscp add demo 10.0.0.8 root secret123
gscp init
gscp run
```

或者：

```bash
gscp run -d
```
