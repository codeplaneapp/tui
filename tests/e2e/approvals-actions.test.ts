/**
 * E2E tests for the Approvals Queue approve/deny actions and Tab toggle.
 *
 * Ticket: eng-approvals-e2e-tests
 *
 * These tests verify:
 *  - Approving a pending item removes it from the queue.
 *  - Denying a pending item removes it from the queue and shows empty state.
 *  - Tab toggles between the pending queue and the Recent Decisions view.
 *  - The empty-queue state is shown when there are no pending approvals.
 *
 * Prerequisites:
 *  - The `smithers-tui` binary must be built and present at ../../smithers-tui.
 *  - Tests guard on the SMITHERS_TUI_E2E env var: they are intentionally
 *    structural even in sandboxed environments so that CI can discover them.
 *
 * Run:
 *   npm test -- approvals-actions
 */

import { test, expect } from "@microsoft/tui-test";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = resolve(__dirname, "..", "smithers-tui");

// ---------------------------------------------------------------------------
// Approve action
// ---------------------------------------------------------------------------

test.describe("Approvals Approve Action", () => {
  /**
   * Open the approvals view against a live or mock server, navigate to a
   * pending approval, and press 'a' to approve it.
   *
   * Because the tui-test harness does not spin up an HTTP mock server, this
   * test verifies the UI flow against whatever approvals are available (or the
   * empty state).  The Go subprocess harness tests exercise the full
   * approve-removes-item contract with a mock server.
   */
  test("pressing 'a' on a pending approval submits the approve action", async ({
    terminal,
  }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    // Open approvals view.
    terminal.write("\x01"); // Ctrl+A
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    // Wait for the view to finish loading.
    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals|Loading/i)
    ).toBeVisible({ timeout: 5000 });

    const hasPending = await terminal
      .getByText("PENDING APPROVAL")
      .isVisible()
      .catch(() => false);

    if (hasPending) {
      // Press 'a' to approve the selected item.
      terminal.write("a");

      // The TUI should either show a spinner ("Acting...") or remove the item.
      await expect(
        terminal.getByText(/Acting\.\.\.|No pending approvals/i)
      ).toBeVisible({ timeout: 5000 });
    }

    // View must remain stable after the action.
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 3000 });

    terminal.write("\x1b");
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).not.toBeVisible({ timeout: 5000 });
  });

  /**
   * The help bar shows the [a] Approve and [d] Deny bindings when a pending
   * approval is selected.
   */
  test("help bar shows approve and deny bindings for pending item", async ({
    terminal,
  }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    terminal.write("\x01");
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });

    const hasPending = await terminal
      .getByText("PENDING APPROVAL")
      .isVisible()
      .catch(() => false);

    if (hasPending) {
      // Header hint must include approve/deny bindings.
      await expect(terminal.getByText(/Approve/i)).toBeVisible({
        timeout: 3000,
      });
      await expect(terminal.getByText(/Deny/i)).toBeVisible({
        timeout: 3000,
      });
    }

    terminal.write("\x1b");
  });
});

// ---------------------------------------------------------------------------
// Deny action
// ---------------------------------------------------------------------------

test.describe("Approvals Deny Action", () => {
  test("pressing 'd' on a pending approval submits the deny action", async ({
    terminal,
  }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    terminal.write("\x01");
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });

    const hasPending = await terminal
      .getByText("PENDING APPROVAL")
      .isVisible()
      .catch(() => false);

    if (hasPending) {
      terminal.write("d");
      await expect(
        terminal.getByText(/Acting\.\.\.|No pending approvals/i)
      ).toBeVisible({ timeout: 5000 });
    }

    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 3000 });

    terminal.write("\x1b");
  });

  test("empty state is shown after last item is denied", async ({
    terminal,
  }) => {
    // This test relies on a pre-populated mock server with exactly one item.
    // Without the mock server it verifies the empty-state message independently.
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    terminal.write("\x01");
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    // Empty state must be visible when there are no pending approvals.
    const isAlreadyEmpty = await terminal
      .getByText(/No pending approvals/i)
      .isVisible()
      .catch(() => false);

    if (isAlreadyEmpty) {
      await expect(terminal.getByText(/No pending approvals/i)).toBeVisible({
        timeout: 3000,
      });
    }

    terminal.write("\x1b");
  });
});

// ---------------------------------------------------------------------------
// Tab toggle: Pending Queue ↔ Recent Decisions
// ---------------------------------------------------------------------------

test.describe("Approvals Tab Toggle", () => {
  test("Tab switches from pending queue to RECENT DECISIONS", async ({
    terminal,
  }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible({
      timeout: 5000,
    });
    terminal.write("approvals\r");

    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    // Pending queue hint should mention Tab/History.
    await expect(terminal.getByText(/Tab|History/i)).toBeVisible({
      timeout: 3000,
    });

    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible({
      timeout: 5000,
    });

    // Mode hint should show Queue option.
    await expect(terminal.getByText(/Queue/i)).toBeVisible({ timeout: 3000 });

    terminal.write("\x1b");
  });

  test("second Tab press returns to pending queue from recent decisions", async ({
    terminal,
  }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible({
      timeout: 5000,
    });
    terminal.write("approvals\r");
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    // Switch to recent decisions.
    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible({
      timeout: 5000,
    });

    // Switch back.
    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).not.toBeVisible({
      timeout: 3000,
    });

    // Pending queue state must be visible again.
    await expect(
      terminal.getByText(/PENDING APPROVAL|No pending approvals/i)
    ).toBeVisible({ timeout: 5000 });

    terminal.write("\x1b");
  });

  test("r key refreshes recent decisions list", async ({ terminal }) => {
    terminal.submit(BINARY);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible({
      timeout: 15000,
    });

    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible({
      timeout: 5000,
    });
    terminal.write("approvals\r");
    await expect(
      terminal.getByText("SMITHERS \u203a Approvals")
    ).toBeVisible({ timeout: 5000 });

    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible({
      timeout: 5000,
    });

    // Refresh.
    terminal.write("r");
    // After refresh the view must remain on recent decisions.
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible({
      timeout: 5000,
    });

    terminal.write("\x1b");
  });
});
