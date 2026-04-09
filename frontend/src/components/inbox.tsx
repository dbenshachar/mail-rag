"use client";

import React, { useEffect, useMemo, useState } from "react";

type EmailSummary = {
  email_id: string;
  subject: string;
  from: string;
  to: string;
  date: string;
  snippet: string;
};

type SearchResult = {
  score: number;
  email: EmailSummary;
};

type InboxProps = {
  apiBase?: string;
};

function toDateLabel(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "Unknown date";
  }
  return parsed.toLocaleString();
}

export function Inbox({ apiBase = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080" }: InboxProps) {
  const normalizedApiBase = useMemo(() => apiBase.replace(/\/$/, ""), [apiBase]);
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [emails, setEmails] = useState<EmailSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshTick, setRefreshTick] = useState(0);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedQuery(query);
    }, 500);
    return () => {
      window.clearTimeout(timer);
    };
  }, [query]);

  useEffect(() => {
    const controller = new AbortController();
    const activeQuery = debouncedQuery.trim();

    async function loadEmails() {
      setLoading(true);
      setError(null);
      try {
        if (activeQuery.length > 0) {
          const searchURL = new URL(`${normalizedApiBase}/api/search`);
          searchURL.searchParams.set("q", activeQuery);
          searchURL.searchParams.set("limit", "20");
          const response = await fetch(searchURL.toString(), { signal: controller.signal });
          if (!response.ok) {
            throw new Error(`Search failed (${response.status})`);
          }
          const payload = (await response.json()) as { results?: SearchResult[] };
          setEmails((payload.results || []).map((item) => item.email));
          return;
        }

        const response = await fetch(`${normalizedApiBase}/api/emails?limit=20&offset=0`, {
          signal: controller.signal
        });
        if (!response.ok) {
          throw new Error(`Fetch failed (${response.status})`);
        }
        const payload = (await response.json()) as { emails?: EmailSummary[] };
        setEmails(payload.emails || []);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") {
          return;
        }
        const message = err instanceof Error ? err.message : "Unknown error";
        setError(message);
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      }
    }

    void loadEmails();

    return () => {
      controller.abort();
    };
  }, [debouncedQuery, normalizedApiBase, refreshTick]);

  async function syncNow() {
    setSyncing(true);
    setError(null);
    try {
      const response = await fetch(`${normalizedApiBase}/api/sync`, {
        method: "POST",
        headers: { "Content-Type": "application/json" }
      });
      if (!response.ok) {
        throw new Error(`Sync failed (${response.status})`);
      }
      setRefreshTick((value) => value + 1);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      setError(message);
    } finally {
      setSyncing(false);
    }
  }

  const modeLabel = debouncedQuery.trim().length > 0 ? `Search: ${debouncedQuery.trim()}` : "Latest emails";

  return (
    <main>
      <div className="inbox-wrap">
        <div className="inbox-head">
          <div>
            <h1 className="inbox-title">Mail RAG</h1>
            <p className="inbox-subtitle">React inbox with semantic retrieval from your Go backend</p>
          </div>
        </div>

        <div className="controls">
          <input
            className="search-input"
            aria-label="Search emails"
            placeholder="Search your inbox"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <button className="sync-btn" onClick={syncNow} disabled={syncing}>
            {syncing ? "Syncing..." : "Sync"}
          </button>
        </div>

        <p className="meta-line">{modeLabel}</p>

        {loading ? <p className="status">Loading emails...</p> : null}
        {error ? (
          <p className="status error" role="alert">
            {error}
          </p>
        ) : null}
        {!loading && !error && emails.length === 0 ? <p className="status">No emails to display.</p> : null}

        {!loading && !error && emails.length > 0 ? (
          <div className="grid" aria-live="polite">
            {emails.map((email) => (
              <article className="card" key={email.email_id}>
                <h2 className="card-subject">{email.subject || "(No subject)"}</h2>
                <p className="card-meta">
                  From {email.from || "Unknown"} to {email.to || "Unknown"} on {toDateLabel(email.date)}
                </p>
                <p className="card-snippet">{email.snippet || "No snippet available."}</p>
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </main>
  );
}
