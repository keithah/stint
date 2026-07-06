"use client";

import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { Providers } from "@/components/providers";
import { ProfileView, type RangeOption } from "@/components/profile-layouts";
import { listProgramLanguages, publicUserProfile, publicUserStats, type PublicProfilePermissions, type StatsRange } from "@/lib/api";
import { languageColorMap } from "@/lib/language-colors";

const ranges: RangeOption[] = [
  { value: "last_7_days", label: "7 days" },
  { value: "last_30_days", label: "30 days" },
  { value: "last_6_months", label: "6 months" },
  { value: "last_year", label: "Year" },
  { value: "all_time", label: "All time" }
];

const defaultPermissions: PublicProfilePermissions = {
  total_time: true,
  projects: true,
  project_visibility: "public_repos",
  languages: true,
  editors: false,
  machines: false,
  operating_systems: false,
  categories: false,
  ai: false,
  summaries: true,
  github: false
};

export default function PublicUserPage() {
  return (
    <Providers>
      <PublicUserContent />
    </Providers>
  );
}

function PublicUserContent() {
  const params = useParams<{ user: string }>();
  const username = params.user;
  const [range, setRangeState] = useState<StatsRange>("last_7_days");
  const [rangeTouched, setRangeTouched] = useState(false);
  const profile = useQuery({ queryKey: ["public-user", username], queryFn: () => publicUserProfile(username), enabled: Boolean(username) });
  const stats = useQuery({ queryKey: ["public-user-stats", username, range], queryFn: () => publicUserStats(username, range), enabled: Boolean(username) });
  const programLanguages = useQuery({ queryKey: ["program-languages"], queryFn: listProgramLanguages, staleTime: 3600000 });

  const publicUser = profile.data?.data ?? stats.data?.user;
  useEffect(() => {
    if (!rangeTouched && publicUser?.default_range) {
      setRangeState(publicUser.default_range);
    }
  }, [publicUser?.default_range, rangeTouched]);
  const setRange = (nextRange: StatsRange) => {
    setRangeTouched(true);
    setRangeState(nextRange);
  };
  const permissions = publicUser?.permissions ?? defaultPermissions;
  const languageColors = useMemo(() => languageColorMap(programLanguages.data?.data ?? []), [programLanguages.data?.data]);

  if (profile.isError || stats.isError) {
    const error = profile.error ?? stats.error;
    return (
      <main className="grid min-h-screen place-items-center px-6">
        <section className="max-w-md rounded-lg border border-line bg-panel p-6 text-center">
          <h1 className="text-xl font-semibold">Public profile unavailable</h1>
          <p className="mt-2 text-sm text-zinc-400">{error?.message ?? "This user has not enabled a public profile."}</p>
        </section>
      </main>
    );
  }

  if (!publicUser) {
    return <main className="grid min-h-screen place-items-center px-6"><p className="text-sm text-zinc-500">Loading profile…</p></main>;
  }

  return (
    <ProfileView
      user={publicUser}
      username={username}
      stats={stats.data?.data}
      permissions={permissions}
      languageColors={languageColors}
      range={range}
      setRange={setRange}
      ranges={ranges}
    />
  );
}
