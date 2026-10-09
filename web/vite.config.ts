import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { readFileSync } from "node:fs";

const version = readFileSync(new URL("../VERSION", import.meta.url), "utf8").trim();
if (!/^[0-9]+\.[0-9]+\.[0-9]+$/.test(version)) throw new Error("Invalid VERSION");

const permissionMode = process.env.FNPROXY_PERMISSION_MODE || "full-ports";
const frontendMode = process.env.FNPROXY_FRONTEND_MODE || "standalone";
if (!["fnos", "standalone", "docker"].includes(frontendMode)) throw new Error("Invalid frontend mode");
if (!["standard", "full-ports"].includes(permissionMode)) throw new Error("Invalid permission mode");

export default defineConfig({
  base: "./",
  plugins: [vue()],
  define: { __APP_VERSION__: JSON.stringify(version), __MIN_LISTEN_PORT__: permissionMode === "full-ports" ? 1 : 1024, __STANDALONE__: frontendMode !== "fnos", __CONTAINER__: frontendMode === "docker" },
  build: { outDir: "dist", emptyOutDir: true },
});
