#!/usr/bin/env node

const childProcess = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const VERSION = "0.1.0";
const PLUGIN_NAME = process.env.STINT_PLUGIN_NAME || "stint-plugin";
const AGENT = process.env.STINT_PLUGIN_AGENT || "";
const HOST = process.env.STINT_PLUGIN_HOST || AGENT || "agent";
const HOST_VERSION_ENV = process.env.STINT_PLUGIN_HOST_VERSION_ENV || "";
const MIN_SYNC_SECONDS = Number.parseInt(process.env.STINT_PLUGIN_MIN_SYNC_SECONDS || "30", 10);

main().catch((error) => {
  log("WARN", error && error.stack ? error.stack : String(error));
  process.exit(0);
});

async function main() {
  if (!AGENT) {
    log("WARN", "STINT_PLUGIN_AGENT is required");
    return;
  }

  if (process.argv.includes("--background")) {
    launchBackground();
    return;
  }

  const input = readInput();
  if (!input) return;
  if (!shouldSync(input)) return;
  if (!claimSyncSlot()) return;

  const stint = await ensureStint();
  await syncActivity(stint, input);
}

function launchBackground() {
  try {
    const stdin = fs.readFileSync(0);
    if (!stdin.length || !stdin.toString("utf8").trim()) return;
    const dir = path.join(os.tmpdir(), `stint-${PLUGIN_NAME}`);
    fs.mkdirSync(dir, { recursive: true });
    const file = path.join(dir, `hook-${Date.now()}-${process.pid}.json`);
    fs.writeFileSync(file, stdin);
    const child = childProcess.spawn(process.execPath, [__filename, file], {
      detached: true,
      stdio: "ignore",
      windowsHide: true,
      env: process.env,
    });
    child.unref();
  } catch (error) {
    log("WARN", error && error.stack ? error.stack : String(error));
  }
}

function readInput() {
  const inputFile = process.argv.find((arg, index) => index > 1 && !arg.startsWith("--"));
  try {
    const raw = inputFile ? fs.readFileSync(inputFile, "utf8") : fs.readFileSync(0, "utf8");
    if (inputFile) {
      try {
        fs.unlinkSync(inputFile);
      } catch (_) {}
    }
    if (!raw.trim()) return undefined;
    return JSON.parse(raw);
  } catch (error) {
    log("WARN", error && error.stack ? error.stack : String(error));
    return undefined;
  }
}

function shouldSync(input) {
  const event = String(input.hook_event_name || input.hookEventName || input.eventName || "");
  if (!event) return true;
  return ["SessionEnd", "UserPromptSubmit", "session_end", "userPromptSubmit"].includes(event);
}

function claimSyncSlot() {
  const minSeconds = Number.isFinite(MIN_SYNC_SECONDS) && MIN_SYNC_SECONDS > 0 ? MIN_SYNC_SECONDS : 30;
  const dir = path.join(os.homedir(), ".wakatime");
  const file = path.join(dir, `${PLUGIN_NAME}.last-sync`);
  const now = Date.now();
  try {
    fs.mkdirSync(dir, { recursive: true });
    const previous = Number.parseInt(fs.readFileSync(file, "utf8"), 10);
    if (Number.isFinite(previous) && now - previous < minSeconds * 1000) {
      log("DEBUG", `Skipping sync; last run was under ${minSeconds}s ago`);
      return false;
    }
  } catch (_) {}

  try {
    fs.writeFileSync(file, String(now));
  } catch (error) {
    log("WARN", error && error.stack ? error.stack : String(error));
  }
  return true;
}

async function syncActivity(stint, input) {
  const cwd = input.cwd || input.projectFolder || process.cwd();
  const hostVersion = HOST_VERSION_ENV ? process.env[HOST_VERSION_ENV] || "unknown" : "unknown";
  const plugin = `${HOST}/${hostVersion} ${PLUGIN_NAME}/${VERSION}`;
  const args = ["--sync-ai-activity", "--ai-agent", AGENT, "--plugin", plugin];
  if (cwd) args.push("--project-folder", cwd);
  await execFile(stint, args, { timeout: 120000, env: childEnv() });
}

async function ensureStint() {
  if (process.env.STINT_BIN && fs.existsSync(process.env.STINT_BIN)) return process.env.STINT_BIN;
  const found = findOnPath("stint");
  if (found) return found;

  const local = path.join(os.homedir(), ".local", "bin", process.platform === "win32" ? "stint.exe" : "stint");
  if (fs.existsSync(local)) return local;

  if (process.env.STINT_PLUGIN_AUTO_INSTALL === "1") {
    if (process.platform === "win32") {
      throw new Error("Stint CLI auto-install is available for macOS and Linux. Install Stint and set STINT_BIN.");
    }
    const installCommand = ["curl", "-fsSL", "https://stint.fyi/install.sh", "|", "sh"].join(" ");
    await execFile("sh", ["-c", installCommand], {
      timeout: 120000,
      env: { ...process.env, STINT_INSTALL_DIR: path.dirname(local) },
    });
    if (!fs.existsSync(local)) {
      throw new Error("Stint CLI auto-install did not create the expected binary. Install Stint manually or set STINT_BIN.");
    }
    return local;
  }

  throw new Error("Stint CLI not found. Install Stint from https://stint.fyi/install.sh, or set STINT_BIN. Set STINT_PLUGIN_AUTO_INSTALL=1 to let this plugin install it.");
}

function findOnPath(name) {
  const paths = String(process.env.PATH || "").split(path.delimiter);
  for (const dir of paths) {
    const full = path.join(dir, name);
    if (fs.existsSync(full)) return full;
  }
  return "";
}

function execFile(command, args, options) {
  return new Promise((resolve) => {
    childProcess.execFile(command, args, { windowsHide: true, ...options }, (error, stdout, stderr) => {
      const output = `${stdout || ""}${stderr || ""}`.trim();
      if (output) log(error ? "WARN" : "DEBUG", output);
      if (error) log("WARN", error.message || String(error));
      resolve();
    });
  });
}

function childEnv() {
  const env = { ...process.env };
  delete env.NODE_OPTIONS;
  return env;
}

function log(level, message) {
  try {
    const dir = path.join(os.homedir(), ".wakatime");
    fs.mkdirSync(dir, { recursive: true });
    fs.appendFileSync(path.join(dir, `${PLUGIN_NAME}.log`), `${new Date().toISOString()} ${level} ${message}\n`);
  } catch (_) {}
}
