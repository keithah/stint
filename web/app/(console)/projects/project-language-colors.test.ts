import { readFileSync } from "node:fs";

const source = readFileSync("app/(console)/projects/[name]/page.tsx", "utf8");

assertIncludes("project detail imports program languages API", source, "listProgramLanguages");
assertIncludes("project detail imports language color map helper", source, "languageColorMap");
assertIncludes("project detail memoizes language colors", source, "const languageColors = useMemo");
assertIncludes("project languages donut receives catalog colors", source, '<SliceDonut title="Languages" rows={data?.stats.languages ?? []} colors={languageColors} />');

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
