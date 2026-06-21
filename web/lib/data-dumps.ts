import type { DataDump } from "./api";

export function dataDumpIsDownloadable(dump: DataDump, now = new Date()) {
  return isCompletedDump(dump.status) && Boolean(dump.download_url) && !dataDumpIsExpired(dump, now);
}

export function dataDumpIsExpired(dump: DataDump, now = new Date()) {
  if (!dump.expires_at) {
    return false;
  }
  const expiresAt = new Date(dump.expires_at);
  if (Number.isNaN(expiresAt.getTime())) {
    return false;
  }
  return expiresAt <= now;
}

export function dataDumpExpiryText(dump: DataDump, now = new Date()) {
  if (!dump.expires_at) {
    return "";
  }
  if (dataDumpIsExpired(dump, now)) {
    return "Expired";
  }
  return `Expires ${new Date(dump.expires_at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric"
  })}`;
}

export function hasPendingDumps(data: { data: DataDump[] } | undefined) {
  return (data?.data ?? []).some((dump) => !isTerminalDumpStatus(dump.status));
}

export function isCompletedDump(status: string) {
  return status.toLowerCase() === "completed";
}

export function isTerminalDumpStatus(status: string) {
  const normalized = status.toLowerCase();
  return normalized === "completed" || normalized === "failed" || normalized === "expired";
}
