import { readFileSync } from "node:fs";

const source = readFileSync("app/(console)/dashboard/page.tsx", "utf8");

assertIncludes("onboarding tells user to open editor", source, "Open your editor");
assertIncludes("onboarding sets activity refresh expectation", source, "within 2 minutes");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
