const GITHUB_RAW_BASE = "https://raw.githubusercontent.com/project-starkite/starkite/main/scripts";

export default {
  async fetch(request) {
    const url = new URL(request.url);

    if (url.pathname === "/install.sh" || url.pathname === "/install") {
      return fetchScript(`${GITHUB_RAW_BASE}/install.sh`);
    }

    if (url.pathname === "/install.ps1") {
      return fetchScript(`${GITHUB_RAW_BASE}/install.ps1`);
    }

    return new Response("Not Found", { status: 404 });
  }
};

async function fetchScript(githubRawUrl) {
  const response = await fetch(githubRawUrl);
  if (!response.ok) {
    return new Response(`Error fetching install script: HTTP ${response.status}`, {
      status: 502,
      headers: { "Content-Type": "text/plain; charset=utf-8" }
    });
  }

  return new Response(response.body, {
    status: 200,
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=300",
      "X-Content-Type-Options": "nosniff"
    }
  });
}
