package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/fastclaw-ai/fastclaw/internal/localagents"
)

func agentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage local FastClaw agent instances",
	}
	cmd.AddCommand(agentsListCmd())
	cmd.AddCommand(agentsInitCmd())
	cmd.AddCommand(agentsStartCmd())
	cmd.AddCommand(agentsStopCmd())
	cmd.AddCommand(agentsLogCmd())
	cmd.AddCommand(agentsConfigCmd())
	cmd.AddCommand(agentsFilesCmd())
	return cmd
}

func agentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List local agent instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			agents, err := localagents.List()
			if err != nil {
				return err
			}
			if len(agents) == 0 {
				fmt.Println("No local agents.")
				return nil
			}
			fmt.Printf("%-20s %-8s %-8s %-7s %-12s %s\n", "NAME", "STATUS", "PID", "PORT", "UPTIME", "HOME")
			for _, ag := range agents {
				status := "stopped"
				pid := "-"
				uptime := "-"
				if ag.Running {
					status = "running"
					pid = strconv.Itoa(ag.PID)
					uptime = ag.Uptime.Round(time.Second).String()
				}
				port := "-"
				if ag.Port > 0 {
					port = strconv.Itoa(ag.Port)
				}
				fmt.Printf("%-20s %-8s %-8s %-7s %-12s %s\n", ag.Name, status, pid, port, uptime, ag.Home)
			}
			return nil
		},
	}
}

func agentsInitCmd() *cobra.Command {
	var opts localagents.InitOptions
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Configure a local agent instance without using the web UI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := localagents.Init(args[0], opts)
			if err != nil {
				return err
			}
			fmt.Printf("Agent %q initialized\n", res.Instance.Name)
			fmt.Printf("Agent ID: %s\n", res.Instance.AgentID)
			fmt.Printf("User ID:  %s\n", res.Instance.UserID)
			fmt.Printf("Home:     %s\n", res.Instance.Home)
			if res.Instance.Port > 0 {
				fmt.Printf("Port:     %d\n", res.Instance.Port)
			}
			if res.ProviderSaved {
				fmt.Println("Provider: saved")
			}
			if res.ModelSaved {
				fmt.Println("Model:    saved")
			}
			if res.CreatedUser && res.GeneratedPassword != "" {
				fmt.Printf("Generated admin password: %s\n", res.GeneratedPassword)
			}
			warnIfAgentRunning(res.Instance.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Home, "home", "", "FASTCLAW_HOME for this agent (default: ~/.fastclaw/local-agents/<name>)")
	cmd.Flags().IntVar(&opts.Port, "port", 0, "default port to use when starting this agent")
	cmd.Flags().StringVar(&opts.AgentName, "agent-name", "", "display name for the created agent")
	cmd.Flags().StringVar(&opts.Description, "description", "", "description for the created agent")
	cmd.Flags().StringVar(&opts.Provider, "provider", "", "provider name, e.g. openai, openrouter, anthropic, ollama")
	cmd.Flags().StringVar(&opts.Model, "model", "", "default model, either <provider>/<model> or <model> with --provider")
	cmd.Flags().StringVar(&opts.APIKeyEnv, "api-key-env", "", "environment variable containing the provider API key")
	cmd.Flags().StringVar(&opts.APIBase, "api-base", "", "provider API base URL")
	cmd.Flags().StringVar(&opts.APIType, "api-type", "", "provider API type (default from provider preset)")
	cmd.Flags().StringVar(&opts.AuthType, "auth-type", "", "provider auth type (default from provider preset)")
	cmd.Flags().StringVar(&opts.Username, "username", "", "admin username to create when the local DB has no users")
	cmd.Flags().StringVar(&opts.Email, "email", "", "admin email to create when the local DB has no users")
	cmd.Flags().StringVar(&opts.Password, "password", "", "admin password to create when the local DB has no users (default: generate)")
	cmd.Flags().StringVar(&opts.DisplayName, "display-name", "", "admin display name")
	cmd.Flags().BoolVar(&opts.SandboxEnabled, "sandbox", false, "enable sandbox for the local instance")
	cmd.Flags().StringVar(&opts.SandboxBackend, "sandbox-backend", "", "sandbox backend, e.g. docker or e2b")
	cmd.Flags().StringVar(&opts.SandboxImage, "sandbox-image", "", "sandbox image/template")
	cmd.Flags().StringVar(&opts.SandboxNetwork, "sandbox-network", "", "sandbox network mode")
	return cmd
}

func agentsStartCmd() *cobra.Command {
	var port int
	var home string
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a local agent instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := localagents.Start(args[0], localagents.StartOptions{
				Port: port,
				Home: home,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Agent %q started (PID %d)\n", inst.Name, inst.PID)
			fmt.Printf("URL:  %s\n", inst.URL)
			fmt.Printf("Home: %s\n", inst.Home)
			fmt.Printf("Logs: %s\n", inst.LogFile)
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port for this agent gateway (default: choose a free port)")
	cmd.Flags().StringVar(&home, "home", "", "FASTCLAW_HOME for this agent (default: ~/.fastclaw/local-agents/<name>)")
	return cmd
}

func agentsStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a local agent instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := localagents.Stop(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Agent %q stopped\n", inst.Name)
			return nil
		},
	}
}

func agentsConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config <name> <get|set> [key] [value]",
		Short: "Read or update a local agent instance config",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			switch args[1] {
			case "get":
				if len(args) > 3 {
					return fmt.Errorf("usage: fastclaw agents config %s get [key]", name)
				}
				key := ""
				if len(args) == 3 {
					key = args[2]
				}
				value, err := localagents.GetConfig(name, key)
				if err != nil {
					return err
				}
				return printValue(value)
			case "set":
				if len(args) != 4 {
					return fmt.Errorf("usage: fastclaw agents config %s set <key> <value>", name)
				}
				if err := localagents.SetConfig(name, args[2], args[3]); err != nil {
					return err
				}
				fmt.Printf("Set %s\n", args[2])
				warnIfAgentRunning(name)
				return nil
			default:
				return fmt.Errorf("unknown config action %q; use get or set", args[1])
			}
		},
	}
}

func agentsFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Manage local agent system files",
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "ls <name>",
		Aliases: []string{"list"},
		Short:   "List configured system files",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := localagents.ListFiles(args[0])
			if err != nil {
				return err
			}
			for _, file := range files {
				fmt.Println(file)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "put <name> <filename> <path>",
		Short: "Write a system file from a local path",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := localagents.PutFile(args[0], args[1], args[2]); err != nil {
				return err
			}
			fmt.Printf("Wrote %s\n", args[1])
			warnIfAgentRunning(args[0])
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <name> <filename> [path]",
		Short: "Read a system file, or write it to a local path",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := localagents.GetFile(args[0], args[1])
			if err != nil {
				return err
			}
			if len(args) == 3 {
				if err := os.WriteFile(args[2], data, 0o644); err != nil {
					return err
				}
				fmt.Printf("Wrote %s\n", args[2])
				return nil
			}
			_, err = os.Stdout.Write(data)
			return err
		},
	})
	return cmd
}

func agentsLogCmd() *cobra.Command {
	var follow bool
	var lines int
	cmd := &cobra.Command{
		Use:     "log <name>",
		Aliases: []string{"logs"},
		Short:   "Show local agent log output",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logFile, err := localagents.LogFile(args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(logFile); os.IsNotExist(err) {
				return fmt.Errorf("no log file found at %s", logFile)
			}
			tailArgs := []string{"-n", fmt.Sprintf("%d", lines)}
			if follow {
				tailArgs = append(tailArgs, "-f")
			}
			tailArgs = append(tailArgs, logFile)

			tailCmd := exec.Command("tail", tailArgs...)
			tailCmd.Stdout = os.Stdout
			tailCmd.Stderr = os.Stderr
			return tailCmd.Run()
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "Number of lines to show")
	return cmd
}

func printValue(value interface{}) error {
	switch v := value.(type) {
	case nil:
		fmt.Println("null")
	case string:
		fmt.Println(v)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}
	return nil
}

func warnIfAgentRunning(name string) {
	st, err := localagents.GetStatus(name)
	if err == nil && st.Running {
		fmt.Fprintln(os.Stderr, "Warning: agent is running; restart it for config changes to take effect.")
	}
}
