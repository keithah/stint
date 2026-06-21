import { dataDumpExpiryText, dataDumpIsDownloadable, hasPendingDumps, isTerminalDumpStatus } from "./data-dumps";
import type { DataDump } from "./api";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

const now = new Date("2026-06-19T12:00:00Z");
const completed: DataDump = {
  id: "dump-1",
  type: "heartbeats",
  status: "Completed",
  percent_complete: 100,
  download_url: "/api/v1/users/current/data_dumps/dump-1/download",
  created_at: "2026-06-19T11:00:00Z"
};

assertEqual("completed dump with future expiry is downloadable", dataDumpIsDownloadable({ ...completed, expires_at: "2026-06-20T12:00:00Z" }, now), true);
assertEqual("completed dump with no expiry is downloadable", dataDumpIsDownloadable(completed, now), true);
assertEqual("completed dump with past expiry is not downloadable", dataDumpIsDownloadable({ ...completed, expires_at: "2026-06-19T11:59:59Z" }, now), false);
assertEqual("processing dump is not downloadable", dataDumpIsDownloadable({ ...completed, status: "Pending", percent_complete: 0 }, now), false);
assertEqual("completed dump without URL is not downloadable", dataDumpIsDownloadable({ ...completed, download_url: "" }, now), false);

assertEqual("pending dumps still poll", hasPendingDumps({ data: [{ ...completed, status: "Pending", percent_complete: 0 }] }), true);
assertEqual("completed expired dumps do not poll", hasPendingDumps({ data: [{ ...completed, expires_at: "2026-06-19T11:00:00Z" }] }), false);
assertEqual("failed status is terminal", isTerminalDumpStatus("Failed"), true);
assertEqual("expired status is terminal", isTerminalDumpStatus("Expired"), true);

assertEqual("future expiry text", dataDumpExpiryText({ ...completed, expires_at: "2026-06-20T12:00:00Z" }, now), "Expires Jun 20, 2026");
assertEqual("past expiry text", dataDumpExpiryText({ ...completed, expires_at: "2026-06-19T11:00:00Z" }, now), "Expired");
assertEqual("missing expiry text", dataDumpExpiryText(completed, now), "");
