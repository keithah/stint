package stintcli

import (
	"fmt"
	"io"
	"strings"
)

func runCustomPricing(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/custom_pricing")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/custom_pricing")
	case "upsert":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/custom_pricing", "usage: stint custom-pricing upsert FILE|--stdin")
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/users/current/custom_pricing", "MODEL", "usage: stint custom-pricing delete MODEL")
	default:
		return fmt.Errorf("unknown custom-pricing command %q", args[0])
	}
}

func runBillingPrefs(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/billing_prefs")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/billing_prefs")
	case "upsert":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/billing_prefs", "usage: stint billing-prefs upsert FILE|--stdin")
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/users/current/billing_prefs", "AGENT", "usage: stint billing-prefs delete AGENT")
	default:
		return fmt.Errorf("unknown billing-prefs command %q", args[0])
	}
}

func runAICosts(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/ai_costs")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/ai_costs")
	case "replace":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/ai_costs", "usage: stint ai-costs replace FILE|--stdin")
	default:
		return fmt.Errorf("unknown ai-costs command %q", args[0])
	}
}
