export const stintInstallCommand = "curl -fsSL https://stint.fyi/install.sh | sh";

export function stintConfiguredInstallCommand(apiURL: string, apiKey: string) {
  return `curl -fsSL https://stint.fyi/install.sh | STINT_API_URL="${apiURL}" STINT_API_KEY="${apiKey}" sh`;
}

const codexMarketplacePlugin = {
  name: "codex-cli-stint",
  commands: [
    "codex plugin marketplace add https://github.com/keithah/stint.git",
    "codex plugin add codex-cli-stint@stint",
  ],
} as const;

const claudeMarketplacePlugin = {
  name: "claude-code-stint",
  commands: [
    "claude plugin marketplace add https://github.com/keithah/stint.git",
    "claude plugin i claude-code-stint@stint",
  ],
} as const;

export const clients = [
  {
    recipeId: "stint-cli-config",
    name: "Stint CLI",
    status: "live",
    description:
      "Install the prebuilt Stint app for terminal, AI agent, and editor activity.",
    bullets: ["One command install", "Works offline", "Adds AI model details"],
  },
  {
    recipeId: "wakatime-cli-config",
    name: "WakaTime CLI",
    status: "supported",
    description:
      "Keep using the official WakaTime CLI and point it at your Stint account.",
    bullets: ["Existing CLI works", "No editor changes", "Offline sync"],
  },
  {
    recipeId: "codex-config",
    name: "Codex",
    status: "supported",
    description:
      "Track Codex CLI and Codex Desktop sessions with Stint-owned setup first.",
    bullets: ["Codex CLI", "Codex Desktop", "Model and token context"],
  },
  {
    recipeId: "claude-code-config",
    name: "Claude Code",
    status: "supported",
    description:
      "Track Claude Code CLI and Claude Desktop coding sessions with Stint.",
    bullets: ["Claude Code CLI", "Claude Desktop", "Model and token context"],
  },
  {
    recipeId: "vscode-config",
    name: "VS Code",
    status: "compatible",
    description:
      "Install the editor extension from the Marketplace, then paste your Stint key.",
    bullets: ["Marketplace install", "Status bar activity", "Language charts"],
  },
  {
    recipeId: "jetbrains-config",
    name: "JetBrains",
    status: "compatible",
    description:
      "Use the JetBrains Marketplace plugin with your Stint endpoint and API key.",
    bullets: ["Marketplace install", "Project charts", "Language charts"],
  },
  {
    recipeId: "vim-config",
    name: "Vim/Neovim",
    status: "compatible",
    description:
      "Install the Vim plugin once, then keep Stint settings in your home folder.",
    bullets: ["Vim plugin", "Neovim support", "Shared config"],
  },
  {
    recipeId: "shell-cli-config",
    name: "Shell CLI",
    status: "compatible",
    description:
      "Send a test heartbeat from any terminal when you want to verify ingestion.",
    bullets: ["No plugin required", "Smoke tests", "Bearer auth"],
  },
] as const;

type SetupOption = {
  title: string;
  badge: string;
  description: string;
  commands?: readonly string[];
  link?: {
    label: string;
    href: string;
  };
};

type SetupStep = {
  title: string;
  body: string;
};

type SetupScreenshot = {
  src: string;
  alt: string;
  caption: string;
};

export type IntegrationConfig = {
  id: string;
  name: string;
  description: string;
  marketplacePlugin?: {
    name: string;
    commands: readonly string[];
  };
  options: readonly SetupOption[];
  steps: readonly SetupStep[];
  verify: readonly string[];
  screenshot: SetupScreenshot;
  notes?: readonly string[];
};

export function integrationConfigs(apiURL: string, apiKey: string) {
  const configuredInstall = stintConfiguredInstallCommand(apiURL, apiKey);
  const configBlock = [
    "mkdir -p ~/.wakatime",
    "cat > ~/.wakatime.cfg <<'EOF'",
    "[settings]",
    `api_url = ${apiURL}`,
    `api_key = ${apiKey}`,
    "heartbeat_rate_limit_seconds = 30",
    "offline = true",
    "EOF",
  ];

  return [
    {
      id: "stint-cli-config",
      name: "Stint CLI",
      description:
        "Best choice if you want Stint's native agent support, offline queue, and WakaTime-compatible heartbeats without building anything.",
      options: [
        {
          title: "Install with one command",
          badge: "Recommended",
          description:
            "Create a Stint key on this page, paste this command in Terminal, and the installer writes ~/.stint.cfg before checking the connection.",
          commands: [configuredInstall],
        },
        {
          title: "Use WakaTime-compatible plugin",
          badge: "Editors",
          description:
            "If you only want editor tracking, install the WakaTime plugin for your editor and use the config below.",
          link: {
            label: "Choose an editor plugin",
            href: "#vscode-config",
          },
        },
        {
          title: "Manual setup",
          badge: "Fallback",
          description:
            "If your shell blocks install scripts, download the matching release asset from GitHub and put the `stint` binary on your PATH.",
          link: {
            label: "Open Stint releases",
            href: "https://github.com/keithah/stint/releases/latest",
          },
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Click the setup button on this page. Stint creates a scoped key and inserts it into the install command.",
        },
        {
          title: "Step 2",
          body: "Run the one command in Terminal. You do not need Go, Git, this repository, or a separate config step.",
        },
        {
          title: "Step 3",
          body: "The installer prints the CLI version and runs doctor. When doctor says connected, keep working normally.",
        },
      ],
      verify: [
        "Run the copied setup command once.",
        "Return here and click Verify connection.",
      ],
      screenshot: {
        src: "/integrations/screenshots/stint-cli.svg",
        alt: "Terminal showing the Stint CLI install command and a successful doctor check",
        caption: "After install, `stint doctor` confirms your key and endpoint.",
      },
      notes: [
        "Use `stint --sync-ai-activity --ai-agent codex` when you want Stint to import local AI coding sessions.",
        "Advanced commands such as `stint api-keys`, `stint data-dumps download DUMP_ID`, `stint custom-rules progress`, and `stint offline sync` remain available from the CLI help.",
      ],
    },
    {
      id: "wakatime-cli-config",
      name: "WakaTime CLI",
      description:
        "Best choice if you already have WakaTime CLI installed and just want it to send activity to Stint.",
      options: [
        {
          title: "Install with one command",
          badge: "CLI",
          description:
            "Install or update the official CLI, then use the Stint config below.",
          commands: [
            "curl -fsSL https://raw.githubusercontent.com/wakatime/wakatime-cli/master/install.sh | sh",
            "wakatime-cli --version",
          ],
        },
        {
          title: "Use WakaTime-compatible plugin",
          badge: "Editors",
          description:
            "Most WakaTime editor plugins install the CLI for you. Install the plugin first, then paste the Stint settings.",
          link: {
            label: "Use VS Code instructions",
            href: "#vscode-config",
          },
        },
        {
          title: "Manual setup",
          badge: "Config file",
          description:
            "Create `~/.wakatime.cfg` yourself. This works for WakaTime CLI and most existing plugins.",
          commands: configBlock,
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Create or copy a Stint integration key.",
        },
        {
          title: "Step 2",
          body: "Save the settings file exactly as shown. The API URL tells WakaTime CLI to send activity to Stint.",
        },
        {
          title: "Step 3",
          body: "Edit a file or run the test command. New activity appears in Stint after the first heartbeat.",
        },
      ],
      verify: [
        ...configBlock,
        'wakatime-cli --entity "$PWD/README.md" --write --plugin shell/1.0.0',
        "wakatime-cli --today",
        "wakatime-cli --offline-count",
        "wakatime-cli --sync-offline-activity",
      ],
      screenshot: {
        src: "/integrations/screenshots/wakatime-cli.svg",
        alt: "Terminal showing a WakaTime CLI heartbeat sent to Stint",
        caption: "The WakaTime CLI can keep its normal command names.",
      },
    },
    {
      id: "codex-config",
      name: "Codex",
      description:
        "Best choice if you use Codex CLI or Codex Desktop and want Stint to show AI coding activity with model and token context.",
      marketplacePlugin: codexMarketplacePlugin,
      options: [
        {
          title: "Install Stint marketplace plugin",
          badge: "Recommended",
          description:
            "Choose Stint marketplace plugin, save your key, then use Codex normally.",
          commands: codexMarketplacePlugin.commands,
        },
        {
          title: "Install Stint CLI",
          badge: "CLI and Desktop",
          description:
            "Install Stint CLI once. It can read local Codex CLI and Codex Desktop activity and send model-aware heartbeats.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use WakaTime-compatible plugin",
          badge: "Compatibility",
          description:
            "If you already use the WakaTime extension in the editor where Codex runs, keep it and point its config at Stint.",
          link: {
            label: "Use WakaTime-compatible VS Code setup",
            href: "#vscode-config",
          },
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install the Stint marketplace plugin for Codex CLI.",
        },
        {
          title: "Step 2",
          body: "Save your Stint endpoint and API key to the WakaTime config file.",
        },
        {
          title: "Step 3",
          body: "Use Codex normally, then open AI Costs or Dashboard to confirm activity.",
        },
      ],
      verify: [
        `stint config init --api-url ${apiURL} --api-key ${apiKey}`,
        "stint --sync-ai-activity --ai-agent codex",
        'stint heartbeat --entity "$PWD/README.md" --category "ai coding" --ai-agent codex --ai-provider openai --ai-model gpt-5-codex --metadata \'{"source":"manual"}\'',
        "stint user-agents",
      ],
      screenshot: {
        src: "/integrations/screenshots/codex.svg",
        alt: "Stint integration guide showing Codex model telemetry",
        caption: "Codex activity shows up with agent, provider, and model fields.",
      },
      notes: [
        "Model-aware fields include `ai_model`, `llm_model`, `ai_provider`, `ai_input_tokens`, `ai_output_tokens`, and structured metadata.",
      ],
    },
    {
      id: "claude-code-config",
      name: "Claude Code",
      description:
        "Best choice if you use Claude Code CLI or Claude Desktop and want Stint to show AI coding sessions with agent, model, token, and project context.",
      marketplacePlugin: claudeMarketplacePlugin,
      options: [
        {
          title: "Install Stint marketplace plugin",
          badge: "Recommended",
          description:
            "Choose Stint marketplace plugin, save your key, then use Claude normally.",
          commands: claudeMarketplacePlugin.commands,
        },
        {
          title: "Install Stint CLI",
          badge: "CLI and Desktop",
          description:
            "Install Stint CLI once. It can read local Claude Code CLI and Claude Desktop activity and send model-aware heartbeats.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use WakaTime-compatible plugin",
          badge: "Compatibility",
          description:
            "If you already use a WakaTime-compatible editor plugin with Claude, keep it and point its config at Stint.",
          link: {
            label: "Use WakaTime-compatible VS Code setup",
            href: "#vscode-config",
          },
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install the Stint marketplace plugin for Claude Code.",
        },
        {
          title: "Step 2",
          body: "Save your Stint endpoint and API key to the WakaTime config file.",
        },
        {
          title: "Step 3",
          body: "Use Claude Code normally, then open AI Costs or Dashboard to confirm activity.",
        },
      ],
      verify: [
        `stint config init --api-url ${apiURL} --api-key ${apiKey}`,
        "stint --sync-ai-activity --ai-agent claude",
        'stint heartbeat --entity "$PWD/README.md" --category "ai coding" --ai-agent claude --ai-provider anthropic --ai-model claude-code --metadata \'{"source":"manual"}\'',
        "stint user-agents",
      ],
      screenshot: {
        src: "/integrations/screenshots/claude-code.svg",
        alt: "Stint integration guide showing Claude Code telemetry",
        caption: "Claude Code CLI and Claude Desktop activity show up with agent, provider, and model fields.",
      },
      notes: [
        "Claude Code support covers CLI and Desktop-style local activity sources through Stint CLI AI sync.",
        "Model-aware fields include `ai_model`, `llm_model`, `ai_provider`, `ai_input_tokens`, `ai_output_tokens`, and structured metadata.",
      ],
    },
    {
      id: "vscode-config",
      name: "VS Code",
      description:
        "Best choice for VS Code users who want Stint-owned setup first, with WakaTime compatibility available.",
      options: [
        {
          title: "Install Stint marketplace plugin",
          badge: "Recommended",
          description:
            "Install Stint for VS Code from the Stint release package while the marketplace listing is being prepared.",
          link: {
            label: "Install Stint for VS Code",
            href: "https://github.com/keithah/stint/releases/latest",
          },
        },
        {
          title: "Install Stint CLI",
          badge: "CLI option",
          description:
            "Use Stint CLI if you prefer terminal setup or need AI activity sync alongside VS Code.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use WakaTime-compatible plugin",
          badge: "Compatibility",
          description:
            "Install from the VS Code Marketplace, then paste your Stint API key when the extension asks.",
          link: {
            label: "Open WakaTime VS Code Marketplace",
            href: "https://marketplace.visualstudio.com/items?itemName=WakaTime.vscode-wakatime",
          },
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install Stint for VS Code, or use the WakaTime-compatible extension if you already have it.",
        },
        {
          title: "Step 2",
          body: "When VS Code asks for an API key, paste your Stint integration key. If using the compatibility path, create the shared config file below.",
        },
        {
          title: "Step 3",
          body: "Edit any file for a minute. Stint will show VS Code in your recent activity.",
        },
      ],
      verify: [
        ...configBlock,
        "Install Stint for VS Code",
        "Or install WakaTime with: code --install-extension WakaTime.vscode-wakatime",
        "Reload VS Code",
        "Open a project and edit a file",
        "Check this page for a VS Code user agent",
      ],
      screenshot: {
        src: "/integrations/screenshots/vscode.svg",
        alt: "VS Code extension setup screen with the Stint API key field highlighted",
        caption: "Paste the Stint key into the extension prompt or shared config file.",
      },
    },
    {
      id: "jetbrains-config",
      name: "JetBrains",
      description:
        "Best choice for IntelliJ IDEA, PyCharm, WebStorm, GoLand, and other JetBrains IDEs with Stint-owned setup first.",
      options: [
        {
          title: "Install Stint marketplace plugin",
          badge: "Recommended",
          description:
            "Install Stint for JetBrains from the Stint release package while the marketplace listing is being prepared.",
          link: {
            label: "Install Stint for JetBrains",
            href: "https://github.com/keithah/stint/releases/latest",
          },
        },
        {
          title: "Install Stint CLI",
          badge: "CLI option",
          description:
            "Use Stint CLI for terminal and AI activity, then add the JetBrains plugin for editor time.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use WakaTime-compatible plugin",
          badge: "Compatibility",
          description:
            "Install from JetBrains Marketplace inside your IDE, then use your Stint key.",
          link: {
            label: "Open WakaTime JetBrains Marketplace",
            href: "https://plugins.jetbrains.com/plugin/7425-wakatime",
          },
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install Stint for JetBrains, or use the WakaTime-compatible plugin if you already have that workflow.",
        },
        {
          title: "Step 2",
          body: "Paste your Stint key when the IDE asks for an API key. If using the compatibility path, create the shared config file below.",
        },
        {
          title: "Step 3",
          body: "Restart the IDE, open a project, and edit a file.",
        },
      ],
      verify: [
        ...configBlock,
        "Install Stint for JetBrains",
        "Or install the WakaTime-compatible JetBrains plugin",
        "Restart your JetBrains IDE",
        "Open a project and edit a file",
        "Check this page for a JetBrains user agent",
      ],
      screenshot: {
        src: "/integrations/screenshots/jetbrains.svg",
        alt: "JetBrains Plugins screen with the WakaTime plugin selected",
        caption: "Install the plugin from inside your JetBrains IDE.",
      },
    },
    {
      id: "vim-config",
      name: "Vim/Neovim",
      description:
        "Best choice for terminal editor users who want the existing Vim plugin to report to Stint.",
      options: [
        {
          title: "Install with one command",
          badge: "CLI option",
          description:
            "Install Stint CLI for terminal checks, then add the Vim plugin for editor activity.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use a marketplace plugin",
          badge: "Plugin",
          description:
            "Install vim-wakatime with your normal Vim plugin manager.",
          link: {
            label: "Open vim-wakatime",
            href: "https://github.com/wakatime/vim-wakatime",
          },
        },
        {
          title: "Manual setup",
          badge: "Config file",
          description:
            "Create the shared config file and restart Vim or Neovim.",
          commands: configBlock,
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install vim-wakatime with your plugin manager.",
        },
        {
          title: "Step 2",
          body: "Save the Stint config file in your home folder.",
        },
        {
          title: "Step 3",
          body: "Open Vim or Neovim and edit a project file.",
        },
      ],
      verify: [
        ...configBlock,
        "Open Vim or Neovim",
        "Edit a tracked file",
        "Run :WakaTimeApiKey if the plugin asks for a key",
      ],
      screenshot: {
        src: "/integrations/screenshots/vim.svg",
        alt: "Vim editing a file with activity sent to Stint",
        caption: "Vim and Neovim use the same home-folder config.",
      },
    },
    {
      id: "shell-cli-config",
      name: "Shell CLI",
      description:
        "Best choice when you want a quick test heartbeat without installing an editor plugin.",
      options: [
        {
          title: "Install with one command",
          badge: "Recommended",
          description:
            "Install Stint CLI for easier terminal tests and offline queue support.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use a marketplace plugin",
          badge: "Optional",
          description:
            "Shell scripts do not need a marketplace plugin. Use your editor's plugin when you want automatic tracking.",
          link: {
            label: "Choose an editor plugin",
            href: "#vscode-config",
          },
        },
        {
          title: "Manual setup",
          badge: "HTTP",
          description:
            "Use curl directly when you only need to confirm that Stint accepts heartbeats.",
          commands: [
            `curl -X POST ${apiURL}/users/current/heartbeats \\`,
            `  -H "Authorization: Bearer ${apiKey}" \\`,
            '  -H "Content-Type: application/json" \\',
            '  -d \'{"entity":"~/src/project/README.md","type":"file","time":1781887600,"project":"project","language":"Markdown"}\'',
          ],
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Create or copy an integration key.",
        },
        {
          title: "Step 2",
          body: "Run the curl command from any terminal.",
        },
        {
          title: "Step 3",
          body: "Refresh Stint and look for the new Shell client in your recent activity.",
        },
      ],
      verify: [
        `curl -X POST ${apiURL}/users/current/heartbeats \\`,
        `  -H "Authorization: Bearer ${apiKey}" \\`,
        '  -H "Content-Type: application/json" \\',
        '  -d \'{"entity":"~/src/project/README.md","type":"file","time":1781887600,"project":"project","language":"Markdown"}\'',
      ],
      screenshot: {
        src: "/integrations/screenshots/shell.svg",
        alt: "Terminal curl command returning a successful heartbeat response",
        caption: "Use curl when you only need to test the API endpoint.",
      },
    },
  ] as const satisfies readonly IntegrationConfig[];
}

export const compatibilityNote =
  "Stint accepts WakaTime-style API keys in Basic auth, Bearer auth, and compatible config files. Existing editor plugins can keep their normal WakaTime names while sending activity to Stint.";
