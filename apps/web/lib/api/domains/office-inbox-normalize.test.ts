import { describe, expect, it } from "vitest";
import { normalizeInboxItem } from "./office-inbox-normalize";

describe("normalizeInboxItem", () => {
  it("maps the backend created_at to createdAt", () => {
    expect(
      normalizeInboxItem({
        id: "item-1",
        type: "approval",
        title: "Hire an agent",
        status: "pending",
        entity_id: "approval-1",
        payload: { a: 1 },
        created_at: "2026-11-20T12:00:00Z",
      }),
    ).toEqual({
      id: "item-1",
      type: "approval",
      title: "Hire an agent",
      status: "pending",
      entity_id: "approval-1",
      payload: { a: 1 },
      createdAt: "2026-11-20T12:00:00Z",
    });
  });

  it("leaves an already normalized item unchanged", () => {
    const item = { id: "i", type: "approval" as const, title: "t", status: "s", createdAt: "x" };
    expect(normalizeInboxItem(item)).toEqual(item);
  });
});
