import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const directory = mkdtempSync(join(tmpdir(), "mailarchive-"));
const platform = process.platform === "win32" ? "windows" : "linux";
const arch = process.arch === "arm64" ? "arm64" : "amd64";
const executable = resolve("dist", `${platform}-${arch}`, platform === "windows" ? "mailarchive.exe" : "mailarchive");
const app = Bun.spawn([executable, "-data-dir", directory, "-port", "0"], {
  stdout: "ignore",
  stderr: "pipe",
});

const timer = setTimeout(() => app.kill(), 15_000);
try {
  let logs = "";
  let url: string | undefined;
  for await (const chunk of app.stderr.pipeThrough(new TextDecoderStream())) {
    logs += chunk;
    url = logs.match(/"url":"(http:\/\/127\.0\.0\.1:\d+)"/)?.[1];
    if (url) break;
  }
  if (!url) throw new Error(`MailArchive wurde nicht erreichbar:\n${logs}`);

  const login = await fetch(`${url}/login`, { signal: AbortSignal.timeout(5000) });
  if (!login.ok || !(await login.text()).includes("Adminzugang einrichten")) {
    throw new Error("Die Anmeldeseite ist nicht erreichbar.");
  }
  const stylesheet = await fetch(`${url}/assets/app.css`, { signal: AbortSignal.timeout(5000) });
  if (!stylesheet.ok || !(await stylesheet.text()).includes("--surface")) {
    throw new Error("Das eingebettete Stylesheet fehlt.");
  }
  console.log(`Starttest für ${platform}/${arch} bestanden.`);
} finally {
  clearTimeout(timer);
  app.kill();
  await app.exited;
  rmSync(directory, { recursive: true, force: true });
}
