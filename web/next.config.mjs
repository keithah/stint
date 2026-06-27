/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  async rewrites() {
    const apiBase = process.env.API_BASE_URL || "http://api:8080";
    return [
      { source: "/@:user", destination: "/users/:user" },
      { source: "/api/:path*", destination: `${apiBase}/api/:path*` },
      { source: "/auth/:path*", destination: `${apiBase}/auth/:path*` }
    ];
  }
};

export default nextConfig;
