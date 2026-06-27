import { NextResponse, type NextRequest } from "next/server";

// Send already-authenticated visitors straight to the dashboard instead of the
// marketing landing page or the login screen. The Go backend sets a 30-day
// `stint_session` cookie on login; its mere presence is enough to skip the
// public entry points. If the cookie is stale/invalid, the dashboard's own
// auth check surfaces a login prompt, so this never traps the user.
export function proxy(request: NextRequest) {
  if (request.cookies.has("stint_session")) {
    const url = request.nextUrl.clone();
    url.pathname = "/dashboard";
    url.search = "";
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  // Only the public entry points; /dashboard and everything else are untouched.
  matcher: ["/", "/login"],
};
