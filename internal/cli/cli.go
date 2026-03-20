package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"gscp/internal/config"
	"gscp/internal/runconfig"
	"gscp/internal/tui"
)

func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "add":
		return runAdd(args[1:])
	case "init":
		return runInit(args[1:])
	case "ls":
		return runList(args[1:])
	case "rm":
		return runRemove(args[1:])
	case "run":
		return runExecute(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAdd(args []string) error {
	if len(args) != 4 {
		return errors.New("usage: gscp add <alias> <host> <username> <password>")
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	server := config.Server{
		Alias:    strings.TrimSpace(args[0]),
		Host:     strings.TrimSpace(args[1]),
		Username: strings.TrimSpace(args[2]),
		Password: args[3],
	}
	if server.Alias == "" || server.Host == "" || server.Username == "" || server.Password == "" {
		return errors.New("alias, host, username and password cannot be empty")
	}

	store.Upsert(server)
	if err := store.Save(); err != nil {
		return err
	}

	path, err := config.Path()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "server %q saved to %s\n", server.Alias, path)
	return nil
}

func runInit(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: gscp init")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	alias := ""
	servers := store.List()
	if len(servers) > 0 {
		alias = servers[0].Alias
	}

	path, err := runconfig.InitInDir(workingDir, alias)
	if err != nil {
		return err
	}

	if alias != "" {
		fmt.Fprintf(os.Stdout, "created default config: %s (active_alias=%s)\n", path, alias)
		return nil
	}

	fmt.Fprintf(os.Stdout, "created default config: %s\n", path)
	return nil
}

func runList(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: gscp ls")
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	servers := store.List()
	if len(servers) == 0 {
		fmt.Fprintln(os.Stdout, "no servers configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tHOST\tUSERNAME")
	for _, server := range servers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", server.Alias, server.Host, server.Username)
	}
	return w.Flush()
}

func runRemove(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: gscp rm <alias>")
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	if err := store.Remove(strings.TrimSpace(args[0])); err != nil {
		if errors.Is(err, config.ErrServerNotFound) {
			return fmt.Errorf("server %q not found", args[0])
		}
		return err
	}

	if err := store.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "server %q removed\n", args[0])
	return nil
}

func runExecute(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: gscp run [-d|env_key]")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	targets, _, err := runconfig.LoadFromDir(workingDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("未发现配置文件")
		}
		return err
	}
	if len(targets) == 0 {
		return errors.New(".genv 中未配置任何环境")
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	envKeys := sortedEnvKeys(targets)
	for _, envKey := range envKeys {
		target := targets[envKey]
		if target.ActiveAlias == "" {
			return fmt.Errorf("env %q missing active_alias", envKey)
		}
		if _, ok := store.Servers[target.ActiveAlias]; !ok {
			return fmt.Errorf("server %q not found", target.ActiveAlias)
		}
	}

	explicitEnv := ""
	if len(args) == 1 {
		arg := strings.TrimSpace(args[0])
		switch arg {
		case "-d":
			explicitEnv, err = selectDefaultEnv(targets)
			if err != nil {
				return err
			}
		default:
			explicitEnv = arg
			if _, ok := targets[explicitEnv]; !ok {
				return fmt.Errorf("env %q not found", explicitEnv)
			}
		}
	}

	return tui.Run(envKeys, targets, store.Servers, explicitEnv, workingDir)
}

func sortedEnvKeys(targets map[string]runconfig.Target) []string {
	envKeys := make([]string, 0, len(targets))
	for envKey := range targets {
		envKeys = append(envKeys, envKey)
	}
	sort.Strings(envKeys)
	return envKeys
}

func selectDefaultEnv(targets map[string]runconfig.Target) (string, error) {
	defaultEnv := ""
	for envKey, target := range targets {
		if !target.IsDefault {
			continue
		}
		if defaultEnv != "" {
			return "", errors.New(".genv 中 is_default=true 只能有一个")
		}
		defaultEnv = envKey
	}
	if defaultEnv == "" {
		return "", errors.New("未找到默认环境，请先在 .genv 中设置 is_default=true")
	}
	return defaultEnv, nil
}

func printUsage() {
	fmt.Fprintln(os.Stdout, `gscp manages remote server profiles for file uploads.

Usage:
  gscp add <alias> <host> <username> <password>
  gscp init
  gscp ls
  gscp rm <alias>
  gscp run
  gscp run <env_key>
  gscp run -d

The run command reads .genv from the current working directory.
Without arguments it opens an interactive environment picker.
With -d it runs the env marked by is_default=true.
Example .genv:
  {
    "dev": {
      "active_alias": "dev-server",
      "is_default": true,
      "local_path": "./dist",
      "to_path": "/var/www/dev",
      "commands": ["cd /var/www/dev", "pm2 restart dev-app"]
    },
    "pro": {
      "active_alias": "prod-server",
      "is_default": false,
      "local_path": "./dist",
      "to_path": "/var/www/prod",
      "commands": ["cd /var/www/prod", "pm2 restart prod-app"]
    }
  }`)
}
