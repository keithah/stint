"use client";

import { KeyRound } from "lucide-react";
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";
import { AICostsCard } from "@/components/settings/ai-costs-card";
import { AccountCard } from "@/components/settings/account-card";
import { ApiKeysCard } from "@/components/settings/api-keys-card";
import { ApiKeysListCard } from "@/components/settings/api-keys-list-card";
import { CustomPricingCard } from "@/components/settings/custom-pricing-card";
import { CustomRulesCard } from "@/components/settings/custom-rules-card";
import { DataExportCard } from "@/components/settings/data-export-card";
import { DiagnosticsCard } from "@/components/settings/diagnostics-card";
import { EditorsCard } from "@/components/settings/editors-card";
import { GitHubAccountCard } from "@/components/settings/github-account-card";
import { OAuthAppsCard } from "@/components/settings/oauth-apps-card";
import { ProfileCard } from "@/components/settings/profile-card";
import { ShareTokensCard } from "@/components/settings/share-tokens-card";
import { WakaTimeImportCard } from "@/components/settings/wakatime-import-card";

export default function SettingsPage() {
  return (
    <Providers>
      <Shell>
        <SettingsContent />
      </Shell>
    </Providers>
  );
}

function SettingsContent() {
  return (
    <div className="mx-auto max-w-5xl px-5 py-6 lg:px-8">
      <header className="mb-8 border-b border-line pb-6">
        <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
          <KeyRound size={14} /> Stint config
        </div>
        <h1 className="text-4xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-2 text-sm text-zinc-400">Manage profile privacy, API keys, editor setup, imports, sharing, and AI cost settings.</p>
      </header>

      <GitHubAccountCard />
      <ProfileCard />
      <ApiKeysCard />
      <DiagnosticsCard />
      <EditorsCard />
      <ApiKeysListCard />
      <OAuthAppsCard />
      <DataExportCard />
      <ShareTokensCard />
      <WakaTimeImportCard />
      <AICostsCard />
      <CustomPricingCard />
      <CustomRulesCard />
      <AccountCard />
    </div>
  );
}
