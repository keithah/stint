import { colorForLanguage, languageColorMap } from "./language-colors";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

const colors = languageColorMap([
  { name: "Ruby", color: "#701516" },
  { name: "TypeScript", color: "#3178c6" }
]);

assertEqual("known language uses catalog color", colorForLanguage("Ruby", colors, 0), "#701516");
assertEqual("matching is case-insensitive", colorForLanguage("typescript", colors, 1), "#3178c6");
assertEqual("unknown language falls back by index", colorForLanguage("Plain Text", colors, 2), "#f97316");
assertEqual("empty color is ignored", colorForLanguage("Go", languageColorMap([{ name: "Go", color: "" }]), 0), "#00b4d8");
