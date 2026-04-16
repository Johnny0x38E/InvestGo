import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
    root: "frontend",
    plugins: [vue()],
    server: {
        host: true,
        port: 5173,
    },
    build: {
        outDir: "dist",
        emptyOutDir: true,
        assetsDir: "assets",
        chunkSizeWarningLimit: 1000, // Raised to 1MB
    },
});
