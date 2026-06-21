export type LanguageColor = {
  name: string;
  color: string;
};

export const fallbackPalette = ["#00b4d8", "#84cc16", "#f97316", "#eab308", "#f43f5e", "#a78bfa"];

export function languageColorMap(languages: LanguageColor[] = []) {
  const colors: Record<string, string> = {};
  for (const language of languages) {
    if (language.name && language.color) {
      colors[language.name.toLowerCase()] = language.color;
    }
  }
  return colors;
}

export function colorForLanguage(name: string, colors: Record<string, string> = {}, index = 0) {
  return colors[name.toLowerCase()] ?? fallbackPalette[index % fallbackPalette.length];
}
