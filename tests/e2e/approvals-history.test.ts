import { test, expect } from "@microsoft/tui-test";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = resolve(__dirname, "..", "smithers-tui");

test.describe("Approvals Recent Decisions", () => {
  test("approvals view shows pending queue by default", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);
    // Open command palette and navigate to approvals view
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible();
    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible();
    terminal.write("approvals\r");
    await expect(terminal.getByText(/SMITHERS.*Approvals/)).toBeVisible();
    // Pending mode hint should be visible
    await expect(
      terminal.getByText(/Tab|History/i)
    ).toBeVisible();
  });

  test("Tab key switches to RECENT DECISIONS view", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible();
    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible();
    terminal.write("approvals\r");
    await expect(terminal.getByText(/SMITHERS.*Approvals/)).toBeVisible();

    // Press Tab to switch to recent decisions
    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible();
    // Mode hint should show "Queue" option
    await expect(terminal.getByText(/Queue/i)).toBeVisible();
  });

  test("Tab key toggles back to pending queue", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible();
    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible();
    terminal.write("approvals\r");
    await expect(terminal.getByText(/SMITHERS.*Approvals/)).toBeVisible();

    // Tab → recent decisions
    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible();

    // Tab again → back to pending queue
    terminal.write("\t");
    await expect(terminal.getByText(/No pending approvals|Pending/)).toBeVisible();
    // RECENT DECISIONS section should no longer be shown
    await expect(terminal.getByText("RECENT DECISIONS")).not.toBeVisible();
  });

  test("Esc exits approvals view", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible();
    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible();
    terminal.write("approvals\r");
    await expect(terminal.getByText(/SMITHERS.*Approvals/)).toBeVisible();

    terminal.write("\x1b");
    // After Esc the approvals header should disappear
    await expect(terminal.getByText(/SMITHERS.*Approvals/)).not.toBeVisible();
  });

  test("recent decisions view shows empty state when no decisions", async ({ terminal }) => {
    terminal.submit(`${BINARY}`);
    await expect(terminal.getByText(/SMITHERS/i)).toBeVisible();
    terminal.write("/");
    await expect(terminal.getByText("approvals")).toBeVisible();
    terminal.write("approvals\r");
    await expect(terminal.getByText(/SMITHERS.*Approvals/)).toBeVisible();

    terminal.write("\t");
    await expect(terminal.getByText("RECENT DECISIONS")).toBeVisible();
    // When no decisions are available, empty state placeholder is shown
    await expect(
      terminal.getByText(/No recent decisions|Loading/)
    ).toBeVisible();
  });
});
