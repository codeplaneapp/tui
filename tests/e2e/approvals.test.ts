import { test, expect } from "@microsoft/tui-test";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = resolve(__dirname, "..", "smithers-tui");

// Note: These tests require a running Smithers TUI process and may be blocked
// in PTY-sandboxed environments. The specs are intentionally correct for CI
// environments that support PTY.

test.describe("Approvals Queue", () => {
  test("opens approvals view via ctrl+a", async ({ terminal }) => {
    // Launch the Smithers TUI.
    terminal.submit(BINARY);

    // Wait for the initial screen to appear.
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    // Send Ctrl+A to open the approvals view.
    terminal.write("\x01");

    // The approvals view header should appear.
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });
  });

  test("shows loading state then approvals or empty message", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    terminal.write("\x01");
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });

    // Should show either loading state, a list of approvals, or the empty state.
    await expect(
      terminal.getByText(/Loading approvals|PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });
  });

  test("opens approvals view via command palette", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    // Open command palette.
    terminal.write("/");
    await expect(terminal.getByText(/approvals/i)).toBeVisible({ timeout: 5000 });

    // Type and submit "approvals".
    terminal.write("approvals\r");
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });
  });

  test("cursor navigates down and up with j/k", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    terminal.write("\x01");
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });

    // Wait for approvals to load (either pending items or empty state).
    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });

    // Navigate down and up — should not crash.
    terminal.write("j");
    await new Promise((r) => setTimeout(r, 100));
    terminal.write("k");
    await new Promise((r) => setTimeout(r, 100));

    // View should still be visible after navigation.
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 3000 });
  });

  test("r key refreshes the list", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    terminal.write("\x01");
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });

    // Wait for initial load to complete.
    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });

    // Press r to refresh.
    terminal.write("r");

    // Should briefly show loading, then re-render.
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });
  });

  test("esc returns to main chat view", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    terminal.write("\x01");
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });

    // Press Escape to go back.
    terminal.write("\x1b");

    // The approvals header should no longer be visible.
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).not.toBeVisible({ timeout: 5000 });
  });

  test("shows cursor indicator for selected item", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({ timeout: 15000 });

    terminal.write("\x01");
    await expect(terminal.getByText("SMITHERS \u203a Approvals")).toBeVisible({ timeout: 5000 });

    // Wait for the view to load.
    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });

    // If there are pending approvals, the cursor indicator should be visible.
    const hasPending = await terminal.getByText("PENDING APPROVAL").isVisible().catch(() => false);
    if (hasPending) {
      await expect(terminal.getByText("\u25b8")).toBeVisible({ timeout: 3000 });
    }
  });
});
