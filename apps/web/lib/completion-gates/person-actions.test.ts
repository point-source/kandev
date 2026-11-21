import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyCriterionAction } from "./person-actions";

const fetchSpy = vi.fn<typeof fetch>();

beforeEach(() => {
  fetchSpy.mockReset();
  fetchSpy.mockImplementation(async () => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

const target = {
  taskId: "t1",
  criterionId: "review",
  criterionRevision: 2,
  subjectRevision: "pr@h1",
};

describe("applyCriterionAction", () => {
  it("routes each action to its endpoint with the heads the person saw", async () => {
    await applyCriterionAction(target, "accept", { reason: "read it", rework: 0, attempts: 0 });
    await applyCriterionAction(target, "send_back", { reason: "fix it", rework: 0, attempts: 0 });
    const urls = fetchSpy.mock.calls.map(([url]) => String(url));
    expect(urls[0]).toMatch(/criteria\/review\/accept$/);
    expect(urls[1]).toMatch(/criteria\/review\/send-back$/);
    expect(JSON.parse(String(fetchSpy.mock.calls[0][1]?.body))).toEqual({
      expected_criterion_revision: 2,
      expected_subject_revision: "pr@h1",
      reason: "read it",
    });
  });

  it("raises only the limit the hold is about", async () => {
    await applyCriterionAction(target, "extend_rounds", { reason: "", rework: 5, attempts: 0 });
    expect(String(fetchSpy.mock.calls[0][0])).toMatch(/criteria\/review\/extend$/);
    expect(JSON.parse(String(fetchSpy.mock.calls[0][1]?.body))).toEqual({
      expected_criterion_revision: 2,
      rework_limit: 5,
      reason: "",
    });
  });
});
