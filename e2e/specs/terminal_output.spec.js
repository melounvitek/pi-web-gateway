import { expect, test } from "@playwright/test";
import { prompts, sessions, tool } from "../support/contract.mjs";
import { expectRunFinished, selectSession, sendPrompt } from "../support/ui.mjs";

test("renders live and restored terminal screen state", async ({ page }) => {
  await page.goto("/");
  await selectSession(page, sessions.terminal);
  await sendPrompt(page, prompts.terminal);

  const card = page.locator(".message--tool-call").filter({ hasText: `$ ${tool.terminalCommand}` }).last();
  await expect(card).toContainText("Terminal frame one");
  await expect(card).toContainText("Terminal frame two");
  await expect(card).not.toContainText("Terminal frame one");
  await expect(card.locator(".terminal-output-run").filter({ hasText: "Terminal frame two" })).toHaveCSS("color", "rgb(0, 205, 0)");
  await expectRunFinished(page);

  await page.reload();
  const restoredCard = page.locator(".message--tool-call").filter({ hasText: `$ ${tool.terminalCommand}` }).last();
  await expect(restoredCard).toContainText("Terminal frame two");
  await expect(restoredCard).not.toContainText("Terminal frame one");
});
