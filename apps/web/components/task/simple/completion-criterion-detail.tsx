"use client";

import { Fragment } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconUser } from "@tabler/icons-react";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import type { CriterionDetail, Reviewer } from "@/lib/completion-gates/criterion-detail";

/** The line under a criterion's name: what decided it, then its commits. */
export function CriterionDetailLine({ detail }: { detail: CriterionDetail }) {
  const { t } = useTranslation();
  const commits = detail.commits.join(", ");
  const described = Boolean(detail.text || detail.reviewers);
  if (!described && !commits) return null;
  return (
    <p
      className="flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground break-all"
      data-testid="completion-criterion-detail"
    >
      {detail.text && <span>{t(detail.text.key, detail.text.values)}</span>}
      {detail.reviewers && (
        <span className="inline-flex flex-wrap items-center gap-x-1">
          <Trans
            i18nKey={detail.reviewers.key}
            components={{ reviewers: <ReviewerList reviewers={detail.reviewers.reviewers} /> }}
          />
        </span>
      )}
      {described && commits && <span aria-hidden="true">·</span>}
      {commits && <span className="font-mono">{commits}</span>}
    </p>
  );
}

function ReviewerList({ reviewers }: { reviewers: Reviewer[] }) {
  return (
    <span className="inline-flex flex-wrap items-center gap-x-1" data-testid="completion-reviewers">
      {reviewers.map((reviewer, index) => (
        <Fragment key={`${reviewer.type}:${reviewer.id}`}>
          {index > 0 && <span aria-hidden="true">,</span>}
          <ReviewerName reviewer={reviewer} />
        </Fragment>
      ))}
    </span>
  );
}

function ReviewerName({ reviewer }: { reviewer: Reviewer }) {
  const { t } = useTranslation();
  const kind =
    reviewer.type === "agent"
      ? t("office:completionGatesReviewerAgent")
      : t("office:completionGatesReviewerPerson");
  return (
    <span className="inline-flex items-center gap-1" data-reviewer-type={reviewer.type}>
      {reviewer.type === "agent" ? (
        <AgentAvatar name={reviewer.name} size="xs" />
      ) : (
        <IconUser className="h-3.5 w-3.5" aria-hidden="true" />
      )}
      <span className="sr-only">{kind}</span>
      <span>{reviewer.name}</span>
    </span>
  );
}
