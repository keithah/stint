import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // Mapped to CSS variables (channels in globals.css) so the theme is
        // changeable in one place; <alpha-value> keeps /opacity modifiers working.
        ink: "rgb(var(--surface) / <alpha-value>)",
        panel: "rgb(var(--surface-2) / <alpha-value>)",
        rail: "rgb(var(--rail) / <alpha-value>)",
        line: "rgb(var(--border) / <alpha-value>)",
        accent: "rgb(var(--accent) / <alpha-value>)",
        ember: "rgb(var(--ember) / <alpha-value>)",
        moss: "rgb(var(--moss) / <alpha-value>)"
      },
      boxShadow: {
        glow: "0 0 0 1px rgba(0, 180, 216, 0.24), 0 24px 70px rgba(0, 0, 0, 0.38)"
      }
    }
  },
  plugins: []
};

export default config;
