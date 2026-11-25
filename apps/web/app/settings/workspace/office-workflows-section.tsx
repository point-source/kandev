"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconBuildingSkyscraper, IconInfoCircle } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsGroupContent, SettingsRow } from "@/components/settings/settings-group";
import { SettingsSection } from "@/components/settings/settings-section";
import { useRouter } from "@/lib/routing/client-router";
import {
  describeOfficeWorkflowProblem,
  type Translate,
} from "@/lib/office-workflow/problem-messages";
import type { Workflow, Workspace } from "@/lib/types/http";
import { WorkflowList } from "./workspace-workflows-list";
import { useOfficeWorkflowItems } from "./use-office-workflow-items";
import {
  useOfficeWorkflowSelection,
  type OfficeWorkflowSelection,
} from "./use-office-workflow-selection";

const EMPTY_SET: ReadonlySet<string> = new Set();

type OfficeWorkflowsSectionProps = {
  workspace: Workspace;
  officeWorkflows: Workflow[];
  canManage: boolean;
};

function OfficeWorkflowPicker({
  selection,
  canManage,
  onSelected,
}: {
  selection: OfficeWorkflowSelection;
  canManage: boolean;
  onSelected: (workflowId: string, wasOffice: boolean) => void;
}) {
  const { t } = useTranslation();
  const candidates = selection.state?.candidates ?? [];
  const selected = selection.state?.selected_workflow_id ?? "";
  const handleChange = async (workflowId: string) => {
    if (workflowId === selected) return;
    const candidate = candidates.find((item) => item.id === workflowId);
    const outcome = await selection.select(workflowId);
    if (outcome === "selected") onSelected(workflowId, candidate?.is_office ?? true);
  };
  return (
    <SettingsGroupContent frame="card" isDirty={false}>
      <SettingsRow
        label={t("workflows:officeWorkflowPickerLabel")}
        description={canManage ? undefined : t("workflows:officeWorkflowPickerNoAccess")}
        controlId="office-workflow-select"
        data-testid="office-workflow-picker"
        controlWrapperClassName="md:w-72"
        control={
          <Select
            value={selected}
            onValueChange={(value) => void handleChange(value)}
            disabled={!canManage || selection.selecting || candidates.length === 0}
          >
            <SelectTrigger
              id="office-workflow-select"
              className="w-full"
              data-testid="office-workflow-select"
            >
              <SelectValue placeholder={t("workflows:officeWorkflowPickerPlaceholder")} />
            </SelectTrigger>
            <SelectContent>
              {candidates.map((candidate) => (
                <SelectItem key={candidate.id} value={candidate.id}>
                  {candidate.built_in
                    ? t("workflows:officeWorkflowBuiltInOption", { name: candidate.name })
                    : candidate.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
    </SettingsGroupContent>
  );
}

function OfficeWorkflowRefusal({ selection }: { selection: OfficeWorkflowSelection }) {
  const { t } = useTranslation();
  if (selection.problems.length === 0 && !selection.error) return null;
  return (
    <Alert variant="destructive" data-testid="office-workflow-refusal">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription>
        <p className="font-medium">{t("workflows:officeWorkflowRefusalTitle")}</p>
        {selection.error && <p className="break-words">{selection.error}</p>}
        <ul className="mt-1 list-disc space-y-1 pl-4">
          {selection.problems.map((problem, index) => (
            <li key={`${problem.code}-${index}`} className="break-words">
              {describeOfficeWorkflowProblem(t, problem)}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

function OfficeWorkflowBadges({
  workflow,
  selectedId,
}: {
  workflow: Workflow;
  selectedId: string;
}) {
  const { t } = useTranslation();
  if (!workflow.is_system && workflow.id !== selectedId) return null;
  return (
    <div
      className="mb-2 flex flex-wrap gap-2"
      data-testid={`office-workflow-badges-${workflow.id}`}
    >
      {workflow.is_system && <Badge variant="outline">{t("workflows:officeBuiltInBadge")}</Badge>}
      {workflow.id === selectedId && (
        <Badge variant="secondary">{t("workflows:officeSelectedBadge")}</Badge>
      )}
    </div>
  );
}

/** Why an Office workflow cannot be deleted from this section, if it cannot. */
function officeDeleteBlockedReason(
  t: Translate,
  workflow: Workflow,
  selectedId: string,
): string | undefined {
  if (workflow.is_system) return describeOfficeWorkflowProblem(t, { code: "built_in_read_only" });
  if (workflow.id === selectedId)
    return describeOfficeWorkflowProblem(t, { code: "selected_delete" });
  return undefined;
}

function BuiltInCopyNotice() {
  const { t } = useTranslation();
  return (
    <Alert className="mb-2" data-testid="office-builtin-copy-notice">
      <IconInfoCircle className="h-4 w-4" />
      <AlertDescription>{t("workflows:officeBuiltInCopyNotice")}</AlertDescription>
    </Alert>
  );
}

/**
 * Office workflows of an Office workspace: the selected-workflow picker, the
 * read-only built-in and editable Office copies.
 */
export function OfficeWorkflowsSection({
  workspace,
  officeWorkflows,
  canManage,
}: OfficeWorkflowsSectionProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const office = useOfficeWorkflowItems(officeWorkflows);
  const selection = useOfficeWorkflowSelection(workspace.id);
  const selectedId = selection.state?.selected_workflow_id ?? workspace.office_workflow_id ?? "";

  const handleSelected = (_workflowId: string, wasOffice: boolean) => {
    // Selecting a Kanban workflow makes it an Office workflow: reload so it
    // moves out of the Kanban list.
    if (!wasOffice) router.refresh();
  };

  return (
    <SettingsSection
      divided
      framed={false}
      icon={<IconBuildingSkyscraper className="h-5 w-5" />}
      title={t("workflows:officeWorkflowsTitle")}
      titleTestId="office-workflows-section-title"
      description={t("workflows:officeWorkflowsDescription")}
    >
      <div className="grid min-w-0 gap-4 pt-4" data-testid="office-workflows-section">
        <OfficeWorkflowPicker
          selection={selection}
          canManage={canManage}
          onSelected={handleSelected}
        />
        <OfficeWorkflowRefusal selection={selection} />
        <WorkflowList
          workflowItems={office.items}
          savedWorkflowItems={office.savedItems}
          orderDirtyIds={EMPTY_SET}
          initialStepsByWorkflowId={office.initialSteps}
          locallyFinalizedWorkflowIds={EMPTY_SET}
          isWorkflowDirty={office.isWorkflowDirty}
          sortable={false}
          renderNotice={(workflow) => (
            <>
              <OfficeWorkflowBadges workflow={workflow} selectedId={selectedId} />
              {office.builtInCopyIds.has(workflow.id) && <BuiltInCopyNotice />}
            </>
          )}
          onUpdate={office.update}
          onDelete={office.remove}
          onDuplicate={office.duplicate}
          onWorkflowSaved={(params) => {
            office.saved(params);
            void selection.reload();
          }}
          onDiscard={office.discard}
          onReorder={() => undefined}
          deleteBlockedReason={(workflow) => officeDeleteBlockedReason(t, workflow, selectedId)}
        />
      </div>
    </SettingsSection>
  );
}
