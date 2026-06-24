"use client";

import { KeyRound } from "lucide-react";
import { PageHeader } from "@/components/ui";
import { AICostsCard } from "@/components/settings/ai-costs-card";
import { AccountCard } from "@/components/settings/account-card";
import { ApiKeysCard } from "@/components/settings/api-keys-card";
import { ApiKeysListCard } from "@/components/settings/api-keys-list-card";
import { BillingPrefsCard } from "@/components/settings/billing-prefs-card";
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
    <SettingsContent />
  );
}

function SettingsContent() {
  return (
    <div className="mx-auto max-w-5xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<KeyRound size={14} />}
        caption="Stint config"
        title="Settings"
        sub="Manage profile privacy, API keys, editor setup, imports, sharing, and AI cost settings."
      />

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
      <BillingPrefsCard />
      <CustomRulesCard />
      <AccountCard />
    </div>
  );
}
