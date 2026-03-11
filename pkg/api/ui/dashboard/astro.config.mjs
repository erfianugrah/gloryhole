import { defineConfig } from "astro/config";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  output: "static",
  outDir: "../static/dist",
  integrations: [react()],
  vite: {
    plugins: [tailwindcss()],
  },
});
