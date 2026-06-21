import { readFileSync } from "node:fs";

const source = readFileSync("components/shell.tsx", "utf8");

assertIncludes("shell uses pathname for active navigation", source, "usePathname");
assertIncludes("shell centralizes nav items", source, "const navItems");
assertIncludes("shell applies active desktop navigation style", source, "desktopNavClass");
assertIncludes("shell applies active mobile navigation style", source, "mobileNavClass");
assertIncludes("shell includes ops brand rail copy", source, "Operations console");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
