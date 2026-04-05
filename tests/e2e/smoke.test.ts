import { test, expect } from "@microsoft/tui-test";

test("shell echo works", async ({ terminal }) => {
  terminal.write("echo tui-test-works\n");
  await expect(terminal.getByText("tui-test-works")).toBeVisible();
});
