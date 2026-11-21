import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  decideProtectedPaths,
  extendCompletionCriterion,
  getEffectiveCompletionCaps,
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
  it("posts a protected-path decision bound to the observed heads", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(SNAPSHOT, { status: 200 }));
    await decideProtectedPaths(
      "t 1",
      "ci_at_head",
      { action: "allow", expected_criterion_revision: 1, expected_subject_revision: "pr@h1" },
      { baseUrl: API },
    );
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(
      `${API}/api/v1/office/tasks/t%201/completion-gate/criteria/ci_at_head/protected-paths`,
    );
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      action: "allow",
      expected_criterion_revision: 1,
      expected_subject_revision: "pr@h1",
    });
  });

  it("posts a cap extension", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(SNAPSHOT, { status: 200 }));
    await extendCompletionCriterion(
      "t1",
      "review_at_head",
      { expected_criterion_revision: 3, max_rework_rounds: 5, reason: "big task" },
      { baseUrl: API },
    );
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(
      `${API}/api/v1/office/tasks/t1/completion-gate/criteria/review_at_head/extend`,
    );
    expect(JSON.parse(String(init?.body))).toMatchObject({
      max_rework_rounds: 5,
      reason: "big task",
    });
  });

  it("reads the effective caps of a task's criteria", async () => {
    const caps = [
      {
        criterion_id: "ci",
        max_verification_attempts: 3,
        max_rework_rounds: 5,
        attempt_count: 1,
        rework_count: 2,
      },
    ];
    fetchSpy.mockResolvedValueOnce(new Response(JSON.stringify({ caps }), { status: 200 }));
    await expect(getEffectiveCompletionCaps("t 1", { baseUrl: API })).resolves.toEqual(caps);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${API}/api/v1/office/tasks/t%201/completion-gate/caps`);
  });
});
