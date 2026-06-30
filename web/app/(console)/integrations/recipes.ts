export const stintInstallCommand = "curl -fsSL https://stint.fyi/install.sh | sh";

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
      "Sync Codex sessions so Stint can show agent, model, project, and cost context.",
    bullets: ["Codex attribution", "Token metadata", "Project detection"],
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
  options: readonly SetupOption[];
  steps: readonly SetupStep[];
  verify: readonly string[];
  screenshot: SetupScreenshot;
  notes?: readonly string[];
};

export function integrationConfigs(apiURL: string, apiKey: string) {
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
            "Open Terminal, paste this command, and Stint installs the right prebuilt binary for your Mac or Linux machine.",
          commands: [stintInstallCommand, "stint --version"],
        },
        {
          title: "Use a marketplace plugin",
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
          body: "Create an integration key on this page. Stint will show the key once, so copy it before leaving.",
        },
        {
          title: "Step 2",
          body: "Run the install command in Terminal. You do not need Go, Git, or this source repository.",
        },
        {
          title: "Step 3",
          body: "Paste your Stint endpoint and key into the config command below. After that, keep working normally.",
        },
      ],
      verify: [
        `stint config init --api-url ${apiURL} --api-key ${apiKey}`,
        "stint doctor",
        'stint heartbeat --entity "$PWD/README.md" --write --project my-project',
        "stint today",
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
          title: "Use a marketplace plugin",
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
        "Best choice if you use Codex and want Stint to show AI coding activity with model and token context.",
      options: [
        {
          title: "Install with one command",
          badge: "Recommended",
          description:
            "Install Stint CLI once. It can read local Codex activity and send model-aware heartbeats.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use a marketplace plugin",
          badge: "Editor",
          description:
            "If you use Codex inside an editor, install that editor's WakaTime plugin too so normal coding time is tracked.",
          link: {
            label: "Use VS Code instructions",
            href: "#vscode-config",
          },
        },
        {
          title: "Manual setup",
          badge: "Advanced",
          description:
            "Send one model-aware heartbeat yourself when you want to test AI telemetry.",
          commands: [
            'stint heartbeat --entity "$PWD/README.md" --category "ai coding" --ai-agent codex --ai-provider openai --ai-model gpt-5-codex --write',
          ],
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install Stint CLI and save your integration key.",
        },
        {
          title: "Step 2",
          body: "Run the Codex sync command after a session. Stint reads local session details and attaches agent metadata.",
        },
        {
          title: "Step 3",
          body: "Open AI Costs or Dashboard to confirm the Codex model, provider, and token fields are visible.",
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
      id: "vscode-config",
      name: "VS Code",
      description:
        "Best choice for VS Code users who want a normal Marketplace install and a simple Stint key paste.",
      options: [
        {
          title: "Install with one command",
          badge: "CLI option",
          description:
            "Use Stint CLI if you prefer terminal setup or need AI activity sync alongside VS Code.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use a marketplace plugin",
          badge: "Recommended",
          description:
            "Install from the VS Code Marketplace, then paste your Stint API key when the extension asks.",
          link: {
            label: "Open VS Code Marketplace",
            href: "https://marketplace.visualstudio.com/items?itemName=WakaTime.vscode-wakatime",
          },
        },
        {
          title: "Manual setup",
          badge: "Config file",
          description:
            "If the extension does not prompt you, create the shared config file below and reload VS Code.",
          commands: [
            ...configBlock,
            "code --install-extension WakaTime.vscode-wakatime",
          ],
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Install the WakaTime extension from the VS Code Marketplace.",
        },
        {
          title: "Step 2",
          body: "When VS Code asks for an API key, paste your Stint integration key.",
        },
        {
          title: "Step 3",
          body: "Edit any file for a minute. Stint will show VS Code under Connection health.",
        },
      ],
      verify: [
        ...configBlock,
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
        "Best choice for IntelliJ IDEA, PyCharm, WebStorm, GoLand, and other JetBrains IDEs.",
      options: [
        {
          title: "Install with one command",
          badge: "CLI option",
          description:
            "Use Stint CLI for terminal and AI activity, then add the JetBrains plugin for editor time.",
          commands: [stintInstallCommand],
        },
        {
          title: "Use a marketplace plugin",
          badge: "Recommended",
          description:
            "Install from JetBrains Marketplace inside your IDE, then use your Stint key.",
          link: {
            label: "Open JetBrains Marketplace",
            href: "https://plugins.jetbrains.com/plugin/7425-wakatime",
          },
        },
        {
          title: "Manual setup",
          badge: "Config file",
          description:
            "Create the shared config file first, then restart the IDE after installing the plugin.",
          commands: configBlock,
        },
      ],
      steps: [
        {
          title: "Step 1",
          body: "Open Settings, then Plugins, then Marketplace. Search for WakaTime and install it.",
        },
        {
          title: "Step 2",
          body: "Paste your Stint key when the IDE asks for a WakaTime API key.",
        },
        {
          title: "Step 3",
          body: "Restart the IDE, open a project, and edit a file.",
        },
      ],
      verify: [
        ...configBlock,
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
          body: "Refresh Stint and look for the new Shell client in Connection health.",
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
