import { readFileSync } from "node:fs";

const source = readFileSync("components/shell.tsx", "utf8");

assertIncludes("shell renders a mobile-only nav", source, "md:hidden");
assertIncludes("mobile nav supports horizontal overflow", source, "overflow-x-auto");
assertIncludes("mobile nav includes dashboard route", source, 'href: "/dashboard"');
assertIncludes("mobile nav includes reports route", source, 'href: "/reports"');
assertIncludes("mobile nav includes settings route", source, 'href: "/settings"');

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
