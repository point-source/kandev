import type { InboxItem } from "@/lib/state/slices/office/types";

export type RawInboxItem = Omit<InboxItem, "createdAt"> & {
  createdAt?: string;
  created_at?: string;
};

export function normalizeInboxItem(raw: RawInboxItem): InboxItem {
  const { created_at: createdAtWire, createdAt, ...item } = raw;
  return { ...item, createdAt: createdAt ?? createdAtWire ?? "" };
}
