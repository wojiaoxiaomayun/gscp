# gscp

[中文说明](./README_ZH.md)

`gscp` is a Go CLI for uploading local files or directories to remote servers and then running remote commands over SSH.

It supports:

- Managing server profiles locally
- Initializing a default `.genv` in the current directory
- Interactive environment selection with Bubble Tea
- Real upload progress UI
- Sequential remote command execution
- `sudo` support through `sudo -S`
- Environment groups via `gscp run -g <group_name>`
- Importing remote server profiles via `gscp add -r <json_url>`

## Build

```bash
go build -o gscp .
```

Or run it directly during development:

```bash
go run . <command>
```

## Commands

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
```

## Server Management

Add one server manually:

```bash
gscp add prod 192.168.1.10 root mypassword
gscp add prod 192.168.1.10:2222 root mypassword
```

If no port is provided, SSH defaults to `22`.

Import server profiles from a remote JSON file:

```bash
gscp add -r https://example.com/servers.json
```

The remote JSON must use the same structure as the local `servers.json` file:

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

Merge rules for `add -r`:

- New aliases are appended to the local config
- Existing aliases are overwritten by the remote config
- Only `http://` and `https://` URLs are accepted

List all saved servers:

```bash
gscp ls
```

Remove a server:

```bash
gscp rm prod
```

Server profiles are stored in the current user's config directory under `gscp/servers.json`.

## Initialize `.genv`

```bash
gscp init
```

This creates a default `.genv` in the current directory.

If you already saved server profiles with `gscp add`, `init` will automatically fill `active_alias` with the first saved alias when possible.

If `.genv` already exists, `init` will not overwrite it.

## `.genv` Format

Example:

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

Fields:

- `groups`: optional named environment lists used by `gscp run -g <group_name>`
- `active_alias`: server alias previously added with `gscp add`
- `is_default`: default environment used by `gscp run -d`
- `local_path`: local file or directory to upload
- `to_path`: remote target directory
- `commands`: commands to run after upload

## Run Deployments

Interactive environment picker:

```bash
gscp run
```

Run one environment directly:

```bash
gscp run pro
```

Run the default environment directly:

```bash
gscp run -d
```

Run a group sequentially:

```bash
gscp run -g prod-all
```

Group runs execute the listed environments one by one in the order they appear in `.genv`.

## What `run` Does

`gscp` will:

1. Read `.genv` from the current directory.
2. Resolve the target environment or group.
3. Look up the saved server by `active_alias`.
4. Connect over SSH.
5. Upload the local file or directory with a Bubble Tea progress UI.
6. Run remote commands in order.
7. Show command output in the TUI log area.

## `sudo` Support

If a command starts with `sudo `, `gscp` automatically rewrites it to use the saved server password with `sudo -S`.

It also requests a TTY for remote command execution so servers that require a TTY for `sudo` can work.

## Notes

- Server passwords are currently stored locally in plain text.
- SSH host key checking currently uses `InsecureIgnoreHostKey`.
- The TUI log view keeps only recent log lines.

## Example Workflow

```bash
gscp add demo 10.0.0.8:2222 root secret123
gscp init
gscp run
```

Or import a remote server list first:

```bash
gscp add -r https://example.com/servers.json
gscp run -g default
```
