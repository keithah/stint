import { readFileSync } from "node:fs";

const source = readFileSync("components/settings/pricing-sources-card.tsx", "utf8");

assertIncludes("pricing source search uses bounded collector", source, "collectPricingRows");
if (source.includes(".filter((m: PricingModel)")) {
  throw new Error("pricing model search should not allocate every match before slicing");
}

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
