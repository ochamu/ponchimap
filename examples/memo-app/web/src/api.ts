const API_BASE = import.meta.env.VITE_API_BASE;

export type Memo = { id: string; body: string; updated_at: number };

export async function listMemos(): Promise<Memo[]> {
  const res = await fetch(`${API_BASE}/memos`);
  if (!res.ok) {
    throw new Error(`listing memos failed: ${res.status}`);
  }
  return res.json();
}

export async function saveMemo(body: string): Promise<string> {
  const res = await fetch(`${API_BASE}/memos`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ body }),
  });
  if (!res.ok) {
    throw new Error(`saving the memo failed: ${res.status}`);
  }
  const { id } = await res.json();
  return id;
}

export async function shareMemo(id: string, to: string): Promise<void> {
  const res = await fetch(`${API_BASE}/memos/${encodeURIComponent(id)}/share`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ to }),
  });
  if (!res.ok) {
    throw new Error(`sharing the memo failed: ${res.status}`);
  }
}
