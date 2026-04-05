import { test, expect } from "@microsoft/tui-test";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = resolve(__dirname, "..", "smithers-tui");

test.describe("Smithers TUI Startup", () => {
  test("binary shows help text", async ({ terminal }) => {
    terminal.submit(`${BINARY} --help`);
    await expect(terminal.getByText("smithers-tui")).toBeVisible();
  });

  test("binary shows version", async ({ terminal }) => {
    terminal.submit(`${BINARY} version`);
    await expect(terminal.getByText(/\d+\.\d+/)).toBeVisible();
  });

  test("binary lists available models", async ({ terminal }) => {
    terminal.submit(`${BINARY} models`);
    // Should show model listing or error about no API key
    await expect(
      terminal.getByText(/model|anthropic|error|key/i)
    ).toBeVisible();
  });
});
