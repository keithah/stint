import { getProjectCommit, listProjectCommits, projectDetail, type Project, type ProjectCommitsResponse } from "./api";

const _nextPageURL: ProjectCommitsResponse["next_page_url"] = "/api/v1/users/current/projects/stint/commits?page=2";
const _prevPageURL: ProjectCommitsResponse["prev_page_url"] = "/api/v1/users/current/projects/stint/commits";
const _projectPublicURL: Project["has_public_url"] = true;
const _projectBadge: Project["badge"] = "https://example.com/badges/stint.svg";
const _commitProjectPublicURL: ProjectCommitsResponse["project"]["has_public_url"] = true;
const _commitProjectBadge: ProjectCommitsResponse["project"]["badge"] = "https://example.com/badges/stint.svg";

type FetchCall = {
  url: string;
  init?: RequestInit;
};

const calls: FetchCall[] = [];
const originalFetch = globalThis.fetch;

globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  calls.push({ url: String(input), init });
  return {
    ok: true,
    status: 200,
    json: async () => ({
      data: { project: { name: "stint" }, stats: { range: "last_year" } },
      commit: { hash: "abcdef123456", truncated_hash: "abcdef1" },
      commits: []
    })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  await projectDetail("stint api", "last_year");
  await listProjectCommits("stint api", { branch: "main branch", page: 2 });
  const commit = await getProjectCommit("stint api", "abc def");
  globalThis.fetch = originalFetch;

  assertEqual("project detail range URL", calls[0]?.url, "/api/v1/users/current/projects/stint%20api?range=last_year");
  assertEqual("project detail method defaults to GET", calls[0]?.init?.method, undefined);
  assertEqual("project commits branch/page URL", calls[1]?.url, "/api/v1/users/current/projects/stint%20api/commits?branch=main+branch&page=2");
  assertEqual("single project commit URL", calls[2]?.url, "/api/v1/users/current/projects/stint%20api/commits/abc%20def");
  assertEqual("single project commit payload", commit.commit.hash, "abcdef123456");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
