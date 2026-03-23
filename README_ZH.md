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
gscp add <alias> <host> <username> <password>
gscp init
gscp ls
gscp rm <alias>
gscp run
gscp run <env_key>
gscp run -d
gscp run -g <group_name>
```

## 服务器管理

添加服务器：

```bash
gscp add prod 192.168.1.10 root mypassword
```

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

## 注意事项

- 服务器密码目前仍然是明文保存在本地配置里
- SSH host key 校验当前使用的是 `InsecureIgnoreHostKey`
- TUI 日志区目前只保留最近一部分日志

## 示例流程

```bash
gscp add demo 10.0.0.8 root secret123
gscp init
gscp run
```

或者执行一个组：

```bash
gscp run -g default
```
