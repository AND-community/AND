// esbuild.js — extension.ts'i tek bir CommonJS paketine (dist/extension.js)
// derler. "vscode" modülü VS Code tarafından çalışma anında sağlanır, bu
// yüzden external bırakılır; jszip gibi npm bağımlılıkları paket içine
// gömülür (kullanıcı node_modules kurmadan .vsix'i kurabilsin diye).

const esbuild = require("esbuild");

const watch = process.argv.includes("--watch");
const production = process.argv.includes("--production");

async function build() {
  const ctx = await esbuild.context({
    entryPoints: ["src/extension.ts"],
    bundle: true,
    outfile: "dist/extension.js",
    external: ["vscode"],
    format: "cjs",
    platform: "node",
    target: "node18",
    sourcemap: !production,
    minify: production,
    logLevel: "info",
  });

  if (watch) {
    await ctx.watch();
  } else {
    await ctx.rebuild();
    await ctx.dispose();
  }
}

build().catch((err) => {
  console.error(err);
  process.exit(1);
});
