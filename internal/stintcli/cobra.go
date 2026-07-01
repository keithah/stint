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
	root.AddCommand(&cobra.Command{
		Use:   "setup",
		Short: "Write Stint and WakaTime-compatible config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commandWantsHelp(args) {
				return cmd.Help()
			}
			return runSetup(args, stdout)
		},
		DisableFlagParsing: true,
	})
	root.AddCommand(&cobra.Command{
		Use:   "collect",
		Short: "Scan local AI agent data and post usage events",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commandWantsHelp(args) {
				return cmd.Help()
			}
			return runCollect(args, stdin, stdout, stderr)
		},
		DisableFlagParsing: true,
	})
	cli := &cobra.Command{Use: "cli", Short: "Manage companion CLIs"}
	cli.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install the pinned upstream wakatime-cli",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commandWantsHelp(args) {
				return cmd.Help()
			}
			return runWakaTimeCLIInstall(args, stdout)
		},
		DisableFlagParsing: true,
	})
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

func commandWantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}
