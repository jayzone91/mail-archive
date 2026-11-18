import { mkdirSync, rmSync } from "node:fs";
import { compile } from "sass";

const host = process.platform === "win32" ? "windows" : "linux";
const assets = "internal/web/assets";
const [action, ...targets] = Bun.argv.slice(2);

function execute(command: string[], env = process.env) {
  const result = Bun.spawnSync(command, { env, stdin: "inherit", stdout: "inherit", stderr: "inherit" });
  if (result.exitCode !== 0) process.exit(result.exitCode ?? 1);
}

function executable(target: string, arch: string) {
  return `dist/${target}-${arch}/mailarchive${target === "windows" ? ".exe" : ""}`;
}

switch (action) {
  case "assets": {
    mkdirSync(assets, { recursive: true });
    const result = await Bun.build({
      entrypoints: ["frontend/ts/app.ts"],
      outdir: assets,
      minify: true,
    });
    if (!result.success) throw new AggregateError(result.logs, "Frontend-Build fehlgeschlagen");
    await Bun.write(`${assets}/app.css`, compile("frontend/scss/app.scss", { style: "compressed" }).css);
    break;
  }
  case "build":
    for (const target of targets.length ? targets : [host]) {
      const arch = targets.length ? "amd64" : process.arch === "arm64" ? "arm64" : "amd64";
      mkdirSync(`dist/${target}-${arch}`, { recursive: true });
      execute(["go", "build", "-trimpath", "-o", executable(target, arch), "./cmd/mailarchive"], {
        ...process.env, GOOS: target, GOARCH: arch, CGO_ENABLED: "0",
      });
    }
    break;
  case "run": {
    const [target = host] = targets;
    if (target !== host) throw new Error(`Der ${target}-Build ist auf diesem Host nicht ausführbar.`);
    const arch = targets.length ? "amd64" : process.arch === "arm64" ? "arm64" : "amd64";
    execute([executable(target, arch)]);
    break;
  }
  case "clean":
    rmSync("dist", { recursive: true, force: true });
    for (const name of ["app.js", "app.css"]) rmSync(`${assets}/${name}`, { force: true });
    break;
  default:
    throw new Error(`Unbekannte Build-Aufgabe: ${action}`);
}
