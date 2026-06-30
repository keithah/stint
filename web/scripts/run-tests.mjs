import { cpus } from "node:os";
import { relative, resolve } from "node:path";
import { readdir } from "node:fs/promises";
import { spawn } from "node:child_process";

const root = resolve(new URL("..", import.meta.url).pathname);
const sucraseNode = resolve(root, "node_modules/.bin/sucrase-node");
const concurrency = Math.max(2, Math.min(8, cpus().length));
const matchFilters = parseMatchFilters(process.argv.slice(2));

async function collectTests(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  const directories = [];
  for (const entry of entries) {
    if (entry.name === "node_modules" || entry.name === ".next") continue;
    const path = resolve(dir, entry.name);
    if (entry.isDirectory()) {
      directories.push(path);
      continue;
    }
    if (/\.test\.tsx?$/.test(entry.name)) {
      files.push(path);
    }
  }
  const nested = await Promise.all(directories.map(collectTests));
  return files.concat(...nested);
}

function runTest(file) {
  return new Promise((resolveRun) => {
    const child = spawn(sucraseNode, [file], { cwd: root, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", (chunk) => {
      output += chunk;
    });
    child.stderr.on("data", (chunk) => {
      output += chunk;
    });
    child.on("close", (code) => {
      resolveRun({ file, code, output });
    });
  });
}

const allTests = (await collectTests(root)).sort();
const tests = matchFilters.length === 0
  ? allTests
  : allTests.filter((file) => {
      const name = relative(root, file);
      return matchFilters.some((filter) => name.includes(filter));
    });
if (tests.length === 0) {
  process.stderr.write(`no test files matched: ${matchFilters.join(", ")}\n`);
  process.exit(1);
}
let next = 0;
let failed = false;

async function worker() {
  while (next < tests.length && !failed) {
    const file = tests[next++];
    const result = await runTest(file);
    const name = relative(root, result.file);
    if (result.code !== 0) {
      failed = true;
      process.stderr.write(`\n${name} failed\n${result.output}`);
      process.exitCode = result.code || 1;
      return;
    }
    const text = result.output.trim();
    if (text) {
      process.stdout.write(`${text}\n`);
    }
  }
}

await Promise.all(Array.from({ length: Math.min(concurrency, tests.length) }, worker));
if (!failed) {
  process.stdout.write(`ran ${tests.length} test files\n`);
}

function parseMatchFilters(args) {
  const filters = [];
  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "--match") {
      if (i + 1 >= args.length) {
        process.stderr.write("--match requires a value\n");
        process.exit(1);
      }
      filters.push(args[++i]);
      continue;
    }
    if (arg.startsWith("--match=")) {
      filters.push(arg.slice("--match=".length));
      continue;
    }
  }
  return filters.filter(Boolean);
}
