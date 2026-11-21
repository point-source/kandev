import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  acceptCriterion,
  allowProtectedFiles,
  extendCompletionCriterion,
  getEffectiveCompletionLimits,
  sendBackCriterion,
} from "./task-completion-gates-api";

const fetchSpy = vi.fn<typeof fetch>();
const API = "http://api.test";
const SNAPSHOT = JSON.stringify({
  task_id: "t1",
  workspace_id: "w",
  revision: 2,
  criteria: [],
  blocked: false,
});

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("completion gate actions", () => {
  it.each([
    ["allow", allowProtectedFiles],
    ["send-back", sendBackCriterion],
    ["accept", acceptCriterion],
  ] as const)("posts a person's %s bound to the observed heads", async (route, action) => {
    fetchSpy.mockResolvedValueOnce(new Response(SNAPSHOT, { status: 200 }));
    await action(
      "t 1",
      "review",
      { expected_criterion_revision: 1, expected_subject_revision: "pr@h1", reason: "why" },
      { baseUrl: API },
    );
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API}/api/v1/office/tasks/t%201/completion-gate/criteria/review/${route}`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      expected_criterion_revision: 1,
      expected_subject_revision: "pr@h1",
      reason: "why",
    });
  });

  it("posts a limit extension", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(SNAPSHOT, { status: 200 }));
    await extendCompletionCriterion(
      "t1",
      "review",
      { expected_criterion_revision: 3, rework_limit: 5, reason: "big task" },
      { baseUrl: API },
    );
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API}/api/v1/office/tasks/t1/completion-gate/criteria/review/extend`);
    expect(JSON.parse(String(init?.body))).toMatchObject({
      rework_limit: 5,
      reason: "big task",
    });
  });

  it("reads the effective limits of a task's criteria", async () => {
    const limits = [
      {
        criterion_id: "ci",
        verification_limit: 3,
        rework_limit: 5,
        attempt_count: 1,
        rework_count: 2,
      },
    ];
    fetchSpy.mockResolvedValueOnce(new Response(JSON.stringify({ limits }), { status: 200 }));
    await expect(getEffectiveCompletionLimits("t 1", { baseUrl: API })).resolves.toEqual(limits);
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${API}/api/v1/office/tasks/t%201/completion-gate/limits`,
    );
  });
});
