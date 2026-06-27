"use client";

import { useParams } from "next/navigation";
import { Providers } from "@/components/providers";
import { PublicSharePage } from "@/components/public-share-page";
import { publicShareStats } from "@/lib/api";

export default function PublicShareUserTokenPage() {
  return (
    <Providers>
      <PublicShareUserTokenContent />
    </Providers>
  );
}

function PublicShareUserTokenContent() {
  const params = useParams<{ userOrToken: string; token: string }>();
  return (
    <PublicSharePage
      queryKey={["public-share", params.userOrToken, params.token]}
      queryFn={() => publicShareStats(params.userOrToken, params.token, "last_7_days")}
      enabled={Boolean(params.userOrToken && params.token)}
    />
  );
}
