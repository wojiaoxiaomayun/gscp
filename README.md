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
- Local web UI for managing servers, workspaces, and `.genv` files via `gscp serve`
- Automatic workspace tracking across `init` and `run` invocations
- File system scanning to discover `.genv` files under configurable roots
- Configurable scan settings (skip directories, scan roots)
- Upload pairs mode: map multiple local paths to different remote directories via `upload_pairs`

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
gscp serve [addr]
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
- `local_path`: local file or directory to upload, supports two formats:
  - String: single file or directory path, e.g. `"./dist"`
  - String array: multiple files or directories, e.g. `["./dist", "./config.json", "./scripts"]`
- `to_path`: remote target directory
- `commands`: commands to run after upload

### Multiple Paths Upload Example

If you need to upload multiple files or directories at once, you can set `local_path` as an array:

```json
{
  "dev": {
    "active_alias": "dev-server",
    "is_default": true,
    "local_path": [
      "./dist",
      "./index.html",
      "./config/production.json",
      "./scripts/deploy.sh"
    ],
    "to_path": "/var/www/app",
    "commands": [
      "cd /var/www/app",
      "chmod +x scripts/deploy.sh",
      "./scripts/deploy.sh"
    ]
  }
}
```

With this configuration, `gscp run` will upload the `dist` directory, `index.html` file, `config/production.json` file, and `scripts/deploy.sh` file to the remote server's `/var/www/app` directory.

### Upload Pairs Mode

Use `upload_pairs` when you need to upload different local paths to **different** remote directories. Each pair has its own `from` (local) and `to` (remote) mapping.

`upload_pairs` takes precedence over `local_path` + `to_path` when both are present.

```json
{
  "prod": {
    "active_alias": "prod-server",
    "upload_pairs": [
      { "from": "./frontend/dist", "to": "/var/www/frontend" },
      { "from": "./backend/bin",   "to": "/opt/app/bin" }
    ],
    "commands": [
      "sudo systemctl restart app"
    ]
  }
}
```

Fields:

- `upload_pairs`: list of `{ "from": "<local path>", "to": "<remote path>" }` entries
  - `from`: local file or directory path (relative to the `.genv` directory or absolute)
  - `to`: remote target directory for this specific pair

Each pair is uploaded sequentially and progress is reported per pair.

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

## Workspace Tracking

Every time you run `gscp init` or `gscp run`, the current working directory is automatically appended to the global `workspaces` list (deduplicated). This lets the web UI discover all project directories that have ever used `gscp`.

## Web UI (`gscp serve`)

Start a local web server to manage server profiles, workspaces, and `.genv` files through a browser:

```bash
gscp serve          # listens on :8080
gscp serve :9090    # custom port
```

Open `http://localhost:8080` in your browser.

### REST API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/servers` | List all saved server profiles |
| `POST` | `/api/servers` | Add or update a server profile |
| `PUT` | `/api/servers/{alias}` | Update a server profile by alias |
| `DELETE` | `/api/servers/{alias}` | Remove a server profile by alias |
| `GET` | `/api/workspaces` | List all recorded workspace paths |
| `POST` | `/api/workspaces/add` | Add a workspace path manually |
| `DELETE` | `/api/workspaces` | Remove a workspace path (body: `{"path":"..."}`) |
| `POST` | `/api/genv/read` | Read and parse a `.genv` file (body: `{"path":"..."}`) |
| `POST` | `/api/genv/write` | Write raw JSON to a `.genv` file (body: `{"path":"...","raw":"..."}`) |
| `GET` | `/api/scan` | Scan for `.genv` files (Server-Sent Events stream) |
| `GET` | `/api/settings` | Get current scan settings |
| `PUT` | `/api/settings` | Update scan settings |

### Scan API (SSE)

`GET /api/scan` streams three event types:

- `scanning` — `{"dir": "<current directory being entered>"}`
- `found` — `{"path": "<directory containing .genv>"}`
- `done` — `{"count": N}`

### Scan Settings

Scan settings control which directories are skipped and which roots are searched:

```json
{
  "skip_dirs": [".git", "node_modules", "vendor", "dist", "build"],
  "scan_roots": ["/home/user/projects"]
}
```

- `skip_dirs`: directory names to skip during scanning. Defaults to a built-in list (`.git`, `node_modules`, `vendor`, `dist`, `build`, etc.).
- `scan_roots`: root paths to scan. Defaults to the user home directory if empty.

Settings are persisted in the global `servers.json` config file.

## Notes
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
