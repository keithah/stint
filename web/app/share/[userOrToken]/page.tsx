"use client";

import { useParams } from "next/navigation";
import { Providers } from "@/components/providers";
import { PublicSharePage } from "@/components/public-share-page";
import { publicShareStatsByToken } from "@/lib/api";

export default function PublicShareTokenPage() {
  return (
    <Providers>
      <PublicShareTokenContent />
    </Providers>
  );
}

function PublicShareTokenContent() {
  const params = useParams<{ userOrToken: string }>();
  const token = params.userOrToken;
  return (
    <PublicSharePage
      queryKey={["public-share-token", token]}
      queryFn={() => publicShareStatsByToken(token, "last_7_days")}
      enabled={Boolean(token)}
    />
  );
}
