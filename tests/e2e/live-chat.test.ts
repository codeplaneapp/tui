/**
 * E2E TUI tests for the Live Chat Viewer feature.
 *
 * These tests exercise the live-chat view from the outside by launching the
 * compiled TUI binary and driving it with keyboard input, then asserting on
 * visible terminal text.
 *
 * Prerequisites:
 *   - The `smithers-tui` binary must be built and present at ../../smithers-tui
 *     relative to the tests/ directory.
 *   - A Smithers server is NOT required: the live chat view falls back to a
 *     static snapshot with a "live streaming unavailable" notice when no server
 *     is running, which is sufficient for all tests here.
 *
 * Run:
 *   npm test -- live-chat
 */

import { test, expect } from "@microsoft/tui-test";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = resolve(__dirname, "..", "smithers-tui");

// ---------------------------------------------------------------------------
// Helper: build the command-palette open sequence.
// The crush TUI opens the command palette with Ctrl+K or '/'.
// ---------------------------------------------------------------------------
const CTRL_K = "\x0b"; // Ctrl+K

test.describe("Live Chat Viewer", () => {
  /**
   * Test 1: Open via command palette and pop back with Esc.
   *
   * Sequence:
   *   1. Launch TUI.
   *   2. Open command palette.
   *   3. Type "chat demo-run" and confirm.
   *   4. Wait for the live-chat header ("SMITHERS › Chat › demo-run").
   *   5. Press Esc and assert the header disappears.
   */
  test("open live chat view and pop back with Esc", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);

    // Wait for TUI to start (shows some initial UI).
    await expect(terminal.getByText(/smithers|crush/i)).toBeVisible({
      timeout: 5000,
    });

    // Open command palette.
    terminal.write(CTRL_K);
    await expect(terminal.getByText(/chat|command/i)).toBeVisible({
      timeout: 3000,
    });

    // Type chat command with a demo run ID.
    terminal.write("chat demo-run\n");

    // The live chat view header should appear (runID truncated to 8 chars).
    await expect(terminal.getByText("demo-run")).toBeVisible({ timeout: 3000 });

    // Press Esc to go back.
    terminal.write("\x1b");

    // Header should no longer show the live chat breadcrumb.
    await expect(terminal.getByText("Chat › demo-run")).not.toBeVisible({
      timeout: 3000,
    });
  });

  /**
   * Test 2: Follow mode toggle via 'f' key.
   *
   * After opening the live chat view, pressing 'f' should toggle follow mode.
   * The help bar reflects the current state with "follow: on" or "follow: off".
   */
  test("follow mode toggle changes help bar text", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);

    await expect(terminal.getByText(/smithers|crush/i)).toBeVisible({
      timeout: 5000,
    });

    terminal.write(CTRL_K);
    await expect(terminal.getByText(/chat|command/i)).toBeVisible({
      timeout: 3000,
    });

    terminal.write("chat demo-run\n");
    await expect(terminal.getByText("demo-run")).toBeVisible({ timeout: 3000 });

    // Default follow mode is ON.
    await expect(terminal.getByText("follow: on")).toBeVisible({
      timeout: 2000,
    });

    // Press 'f' — follow mode should turn OFF.
    terminal.write("f");
    await expect(terminal.getByText("follow: off")).toBeVisible({
      timeout: 2000,
    });

    // Press 'f' again — follow mode should turn ON.
    terminal.write("f");
    await expect(terminal.getByText("follow: on")).toBeVisible({
      timeout: 2000,
    });

    // Clean up.
    terminal.write("\x1b");
  });

  /**
   * Test 3: Scroll keys disable follow mode.
   *
   * When follow mode is ON and the user presses the Up arrow,
   * follow mode should be disabled.
   */
  test("up arrow disables follow mode", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);

    await expect(terminal.getByText(/smithers|crush/i)).toBeVisible({
      timeout: 5000,
    });

    terminal.write(CTRL_K);
    await expect(terminal.getByText(/chat|command/i)).toBeVisible({
      timeout: 3000,
    });

    terminal.write("chat demo-run\n");
    await expect(terminal.getByText("demo-run")).toBeVisible({ timeout: 3000 });
    await expect(terminal.getByText("follow: on")).toBeVisible({
      timeout: 2000,
    });

    // Press Up arrow — follow should turn off.
    terminal.write("\x1b[A"); // ANSI Up arrow
    await expect(terminal.getByText("follow: off")).toBeVisible({
      timeout: 2000,
    });

    terminal.write("\x1b");
  });

  /**
   * Test 4: Help bar shows hijack binding.
   *
   * The live chat view should always show the 'h' hijack binding in the help bar.
   */
  test("help bar shows hijack binding", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);

    await expect(terminal.getByText(/smithers|crush/i)).toBeVisible({
      timeout: 5000,
    });

    terminal.write(CTRL_K);
    await expect(terminal.getByText(/chat|command/i)).toBeVisible({
      timeout: 3000,
    });

    terminal.write("chat demo-run\n");
    await expect(terminal.getByText("demo-run")).toBeVisible({ timeout: 3000 });

    // Help bar should include "hijack".
    await expect(terminal.getByText(/hijack/i)).toBeVisible({ timeout: 2000 });

    terminal.write("\x1b");
  });

  /**
   * Test 5: 'q' key pops the view (same as Esc).
   */
  test("q key pops live chat view", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);

    await expect(terminal.getByText(/smithers|crush/i)).toBeVisible({
      timeout: 5000,
    });

    terminal.write(CTRL_K);
    await expect(terminal.getByText(/chat|command/i)).toBeVisible({
      timeout: 3000,
    });

    terminal.write("chat demo-run\n");
    await expect(terminal.getByText("demo-run")).toBeVisible({ timeout: 3000 });

    terminal.write("q");
    await expect(terminal.getByText("Chat › demo-run")).not.toBeVisible({
      timeout: 3000,
    });
  });
});
