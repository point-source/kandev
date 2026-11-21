import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import type { OfficeApiClient } from "./office-api-client";
import {
  chooseProject,
  createAttachmentProject,
  openNewTaskDialog,
} from "../tests/office/new-task-attachments-helpers";

/** Saves workspace defaults (CI with protected paths, plus review) from the settings card. */
export async function saveGateDefaults(page: Page) {
  await page.goto("/office/workspace/settings");
  const card = page.getByTestId("completion-gates-card");
  await card.getByTestId("settings-criteria-ci").click();
  await card.getByTestId("settings-criteria-protected").fill("tests/**, .github/**");
  await card.getByTestId("settings-criteria-review").click();
  await card.getByTestId("completion-gates-save").click();
  await expect(page.getByText("Completion gate settings saved")).toBeVisible();
  await page.reload();
  await expect(
    page.getByTestId("completion-gates-card").getByTestId("settings-criteria-protected"),
  ).toHaveValue("tests/**, .github/**");
}

/** Opens the dialog in a repository-less project and explicitly requests CI only. */
export async function createWithCriteriaWithoutRepository(
  page: Page,
  officeApi: OfficeApiClient,
  workspaceId: string,
  mobile: boolean,
) {
  const project = await createAttachmentProject(officeApi, workspaceId, "Gate criteria");
  const dialog = await openNewTaskDialog(page, mobile);
  await dialog.getByPlaceholder("Task title").fill("Gated without repo");
  await chooseProject(page, dialog, project.name);
  const group = dialog.getByTestId("new-task-completion-criteria");
  await expect(group).toContainText("(CI check (GitHub), Review)");
  await group.getByRole("button").first().click();
  await expect(group.getByTestId("new-task-criteria-ci")).toBeChecked();
  // An explicit choice (CI only) is validated; untouched defaults that cannot
  // apply are skipped instead.
  await group.getByTestId("new-task-criteria-review").click();
  await dialog.getByTestId("new-task-create-button").click();
  await expect(page.getByText("CI and review criteria need a repository")).toBeVisible();
}
