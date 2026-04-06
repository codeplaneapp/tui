import { test, expect } from "@microsoft/tui-test";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";
import { mkdtempSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = resolve(__dirname, "..", "smithers-tui");

test("binary starts and shows something", async ({ terminal }) => {
  // First just verify the binary can start in a real PTY
  const configDir = mkdtempSync(join(tmpdir(), "crush-e2e-cfg-"));
  const dataDir = mkdtempSync(join(tmpdir(), "crush-e2e-dat-"));
  writeFileSync(
    join(configDir, "smithers-tui.json"),
    JSON.stringify({
      smithers: {
        dbPath: ".smithers/smithers.db",
        workflowDir: ".smithers/workflows",
      },
    })
  );

  // Try writing the command directly
  terminal.write(
    `SMITHERS_TUI_GLOBAL_CONFIG="${configDir}" SMITHERS_TUI_GLOBAL_DATA="${dataDir}" "${BINARY}"\n`
  );

  // Wait for ANY output
  await new Promise((r) => setTimeout(r, 5000));
  const buf = terminal.getBuffer();
  console.log("Buffer length:", buf.length);
  console.log("Buffer content (first 500):", buf.slice(0, 500));
});
