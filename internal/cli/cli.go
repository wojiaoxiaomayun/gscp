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
	if len(args) > 2 {
		return errors.New("usage: gscp run [-d|env_key|-g group_name]")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	cfg, _, err := runconfig.LoadConfigFromDir(workingDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("未发现配置文件")
		}
		return err
	}
	if len(cfg.Targets) == 0 {
		return errors.New(".genv 中未配置任何环境")
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	envKeys := sortedEnvKeys(cfg.Targets)
	for _, envKey := range envKeys {
		target := cfg.Targets[envKey]
		if target.ActiveAlias == "" {
			return fmt.Errorf("env %q missing active_alias", envKey)
		}
		if _, ok := store.Servers[target.ActiveAlias]; !ok {
			return fmt.Errorf("server %q not found", target.ActiveAlias)
		}
	}

	explicitEnv := ""
	if len(args) == 0 {
		return tui.Run(envKeys, cfg.Targets, store.Servers, explicitEnv, workingDir, false)
	}

	arg := strings.TrimSpace(args[0])
	switch arg {
	case "-d":
		if len(args) != 1 {
			return errors.New("usage: gscp run -d")
		}
		explicitEnv, err = selectDefaultEnv(cfg.Targets)
		if err != nil {
			return err
		}
		return tui.Run(envKeys, cfg.Targets, store.Servers, explicitEnv, workingDir, false)
	case "-g":
		if len(args) != 2 {
			return errors.New("usage: gscp run -g <group_name>")
		}
		return runGroup(strings.TrimSpace(args[1]), cfg, store.Servers, workingDir)
	default:
		if len(args) != 1 {
			return errors.New("usage: gscp run <env_key>")
		}
		explicitEnv = arg
		if _, ok := cfg.Targets[explicitEnv]; !ok {
			return fmt.Errorf("env %q not found", explicitEnv)
		}
		return tui.Run(envKeys, cfg.Targets, store.Servers, explicitEnv, workingDir, false)
	}
}

func runGroup(groupName string, cfg *runconfig.ConfigFile, servers map[string]config.Server, workingDir string) error {
	members, ok := cfg.Groups[groupName]
	if !ok {
		return fmt.Errorf("group %q not found", groupName)
	}
	if len(members) == 0 {
		return fmt.Errorf("group %q has no environments", groupName)
	}

	envKeys := sortedEnvKeys(cfg.Targets)
	for index, envKey := range members {
		envKey = strings.TrimSpace(envKey)
		if envKey == "" {
			return fmt.Errorf("group %q contains an empty environment name", groupName)
		}
		if _, ok := cfg.Targets[envKey]; !ok {
			return fmt.Errorf("group %q references unknown env %q", groupName, envKey)
		}

		fmt.Fprintf(os.Stdout, "running group %q (%d/%d): %s\n", groupName, index+1, len(members), envKey)
		if err := tui.Run(envKeys, cfg.Targets, servers, envKey, workingDir, true); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stdout, "group %q finished\n", groupName)
	return nil
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
  gscp run -g <group_name>

The run command reads .genv from the current working directory.
Without arguments it opens an interactive environment picker.
With -d it runs the env marked by is_default=true.
With -g it runs all envs in the named group sequentially.
Example .genv:
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
      "commands": ["cd /var/www/dev", "pm2 restart dev-app"]
    }
  }`)
}
