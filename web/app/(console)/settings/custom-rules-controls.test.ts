import { readFileSync } from "node:fs";

const source = readFileSync("components/settings/custom-rules-card.tsx", "utf8");
const apiSource = readFileSync("lib/api.ts", "utf8");

assertIncludes("settings imports custom rule delete helper", source, "deleteCustomRule");
assertIncludes("settings exposes action selector", source, "ruleAction");
assertIncludes("settings exposes source selector", source, "ruleSource");
assertIncludes("settings exposes operation selector", source, "ruleOperation");
assertIncludes("settings exposes regex operation", source, 'value: "regex", label: "Regex"');
assertIncludes("settings exposes destination selector", source, "ruleDestination");
assertIncludes("settings deletes individual rules", source, "removeCustomRule.mutate");
assertIncludes("API client has custom rule delete function", apiSource, "deleteCustomRule");
assertIncludes("API client calls custom rule delete endpoint", apiSource, "/api/v1/users/current/custom_rules/");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
