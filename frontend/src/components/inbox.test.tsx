import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Inbox } from "./inbox";

function jsonResponse(payload: unknown): Promise<Response> {
  return Promise.resolve(
    new Response(JSON.stringify(payload), {
      status: 200,
      headers: { "Content-Type": "application/json" }
    })
  );
}

async function waitMs(ms: number): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Inbox", () => {
  it("fires one debounced search request after 500ms of idle typing", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/search")) {
        return jsonResponse({ query: "abc", results: [] });
      }
      return jsonResponse({ emails: [], total: 0 });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<Inbox apiBase="http://api.test" />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    const input = screen.getByLabelText("Search emails");
    fireEvent.change(input, { target: { value: "a" } });
    fireEvent.change(input, { target: { value: "ab" } });
    fireEvent.change(input, { target: { value: "abc" } });

    await waitMs(300);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await waitMs(260);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(String(fetchMock.mock.calls[1][0])).toContain("/api/search?q=abc");
  });

  it("clearing the search input loads default emails endpoint", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/search")) {
        return jsonResponse({ query: "job", results: [] });
      }
      return jsonResponse({ emails: [], total: 0 });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<Inbox apiBase="http://api.test" />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    const input = screen.getByLabelText("Search emails");

    fireEvent.change(input, { target: { value: "job" } });
    await waitMs(560);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(String(fetchMock.mock.calls[1][0])).toContain("/api/search?q=job");

    fireEvent.change(input, { target: { value: "" } });
    await waitMs(560);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(String(fetchMock.mock.calls[2][0])).toContain("/api/emails?limit=20&offset=0");
  });

  it("shows loading and empty state", async () => {
    let resolveFetch: ((value: Response) => void) | null = null;
    const pending = new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    });
    const fetchMock = vi.fn(() => pending);
    vi.stubGlobal("fetch", fetchMock);

    render(<Inbox apiBase="http://api.test" />);

    expect(screen.getByText("Loading emails...")).toBeInTheDocument();

    if (resolveFetch) {
      resolveFetch(
        new Response(JSON.stringify({ emails: [], total: 0 }), {
          status: 200,
          headers: { "Content-Type": "application/json" }
        })
      );
    }

    await waitFor(() => expect(screen.getByText("No emails to display.")).toBeInTheDocument());
  });

  it("shows error state when request fails", async () => {
    const fetchMock = vi.fn(() => Promise.reject(new Error("network down")));
    vi.stubGlobal("fetch", fetchMock);

    render(<Inbox apiBase="http://api.test" />);

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("network down"));
  });
});
