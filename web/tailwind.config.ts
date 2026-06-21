import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#0d0d0d",
        panel: "#171717",
        rail: "#242424",
        line: "#303030",
        accent: "#00b4d8",
        ember: "#f97316",
        moss: "#84cc16"
      },
      boxShadow: {
        glow: "0 0 0 1px rgba(0, 180, 216, 0.24), 0 24px 70px rgba(0, 0, 0, 0.38)"
      }
    }
  },
  plugins: []
};

export default config;
