package stintcli

import (
	"io"

	"github.com/spf13/cobra"
)

func runCobraCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error) {
	if len(args) == 0 || !cobraManagedCommand(args[0]) {
		return false, nil
	}
	cmd := newCobraRoot(stdin, stdout, stderr)
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return true, cmd.Execute()
}

func newCobraRoot(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "stint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	setup := &cobra.Command{
		Use:   "setup",
		Short: "Write Stint and WakaTime-compatible config",
		RunE: withHelp(func(args []string) error {
			return runSetup(args, stdout)
		}),
		DisableFlagParsing: true,
	}
	addSetupHelpFlags(setup)
	root.AddCommand(setup)

	collect := &cobra.Command{
		Use:   "collect",
		Short: "Scan local AI agent data and post usage events",
		RunE: withHelp(func(args []string) error {
			return runCollect(args, stdin, stdout, stderr)
		}),
		DisableFlagParsing: true,
	}
	addCollectHelpFlags(collect)
	root.AddCommand(collect)

	cli := &cobra.Command{Use: "cli", Short: "Manage companion CLIs"}
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the pinned upstream wakatime-cli",
		RunE: withHelp(func(args []string) error {
			return runWakaTimeCLIInstall(args, stdout)
		}),
		DisableFlagParsing: true,
	}
	cli.AddCommand(install)
	root.AddCommand(cli)
	return root
}

func cobraManagedCommand(command string) bool {
	switch command {
	case "setup", "cli", "collect":
		return true
	default:
		return false
	}
}

func withHelp(run func(args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if commandWantsHelp(args) {
			return cmd.Help()
		}
		return run(args)
	}
}

func commandWantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func addSetupHelpFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.String("server", "", "Stint API URL")
	flags.String("key", "", "Stint API key")
	flags.String("stint-config", DefaultStintConfigPath(), "native Stint config path")
	flags.String("wakatime-config", DefaultWakaTimeConfigPath(), "WakaTime compatibility config path")
}

func addCollectHelpFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.String("api-url", "", "Stint API base URL")
	flags.String("api-key", "", "Stint API key")
	flags.String("cost-mode", "", "cost mode hint (calculate|provided)")
	flags.String("state", "", "incremental state file path")
	flags.String("interval", "", "poll interval when --watch is set")
	flags.String("agent", "", "scan only this agent id")
	flags.Bool("once", true, "run a single scan and exit")
	flags.Bool("watch", false, "poll loop: re-scan on an interval")
	flags.Bool("dry-run", false, "scan and report only; do not POST")
	flags.String("config", "~/.stint/collect.json", "collector config file path")
	flags.Bool("print-config", false, "print the resolved config and exit")
	flags.Bool("init-config", false, "write a starter config if absent, then exit")
}
