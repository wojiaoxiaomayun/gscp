# gscp

[中文说明](./README_ZH.md)

`gscp` is a Go CLI tool for uploading local files to remote servers and then running remote commands.

It supports:

- Managing server profiles locally
- Initializing a default `.genv` config in the current directory
- Picking an environment interactively with Bubble Tea
- Uploading files with progress display
- Running remote commands over SSH
- Supporting `sudo` commands with `sudo -S`

## Build

```bash
go build -o gscp .
```

Or run it directly during development:

```bash
go run . <command>
```

## Commands

### Add a server

```bash
gscp add <alias> <host> <username> <password>
```

Example:

```bash
gscp add prod 192.168.1.10 root mypassword
```

This saves the server config locally.

### List servers

```bash
gscp ls
```

### Remove a server

```bash
gscp rm <alias>
```

### Initialize a default `.genv`

```bash
gscp init
```

This creates a `.genv` file in the current working directory.

If you already saved server profiles with `add`, `init` will try to fill `active_alias` automatically with the first saved alias.

If `.genv` already exists, `init` will not overwrite it.

## `.genv` format

Example:

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

Fields:

- `active_alias`: server alias previously added with `gscp add`
- `is_default`: default environment used by `gscp run -d`
- `local_path`: local file or directory to upload
- `to_path`: remote target directory
- `commands`: commands to run after upload

## Run deployment

### Interactive environment selection

```bash
gscp run
```

This opens a Bubble Tea UI and lets you choose an environment.

### Run a specific environment directly

```bash
gscp run <env_key>
```

Example:

```bash
gscp run pro
```

### Run the default environment directly

```bash
gscp run -d
```

This uses the environment whose `is_default` is `true`.

Rules:

- If no environment has `is_default=true`, `gscp run -d` fails
- If more than one environment has `is_default=true`, `gscp run -d` fails

## What happens during `run`

`gscp` will:

1. Read `.genv` from the current directory
2. Select the target environment
3. Find the matching saved server profile by `active_alias`
4. Connect over SSH
5. Upload the local file or directory with a progress UI
6. Run remote commands in order

Remote command output is shown in the TUI log area.

## `sudo` support

If a command starts with `sudo `, `gscp` will automatically try to use the saved server password with `sudo -S`.

Example:

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

Notes:

- `gscp` also requests a TTY for remote commands, which helps with servers that require a TTY for `sudo`
- If the server has a custom sudo policy, you may still need to adjust it

## Notes

- Server passwords are currently stored locally in plain text in the user config file
- SSH host key checking currently uses `InsecureIgnoreHostKey`
- Command output is visible in the Bubble Tea log panel
- The log panel currently keeps recent lines only

## Local config location

Server profiles are stored in the current user's config directory under:

```text
gscp/servers.json
```

## Example workflow

```bash
gscp add demo 10.0.0.8 root secret123
gscp init
gscp run
```

Or:

```bash
gscp run -d
```
