import { readFileSync } from "node:fs";

const source = readFileSync("app/share/[userOrToken]/[token]/page.tsx", "utf8");

assertIncludes("program language API client", source, "listProgramLanguages");
assertIncludes("language color map helper", source, "languageColorMap");
assertIncludes("languages donut receives catalog colors", source, '<SliceDonut title="Languages" rows={data?.languages ?? []} colors={languageColors} />');

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected page source to include ${needle}`);
  }
}
