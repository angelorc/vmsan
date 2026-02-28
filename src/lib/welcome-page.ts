/**
 * Template generators for the node22-demo runtime welcome page.
 * All functions are pure and return string content ready to write to files.
 */

export function generateWelcomeHtml(vmId: string, ports: number[]): string {
  const portList = ports.map((p) => `<li>${p}</li>`).join("\n            ");
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>vmsan VM ${vmId}</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background: #0f172a;
      color: #e2e8f0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 2rem;
    }
    .container { max-width: 640px; width: 100%; }
    .header { text-align: center; margin-bottom: 2rem; }
    .logo {
      font-size: 2.5rem;
      font-weight: 800;
      background: linear-gradient(135deg, #f97316, #ef4444);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      background-clip: text;
    }
    .subtitle { color: #94a3b8; margin-top: 0.5rem; font-size: 1.1rem; }
    .card {
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 12px;
      padding: 1.5rem;
      margin-bottom: 1.25rem;
    }
    .card h2 { font-size: 1rem; color: #f97316; margin-bottom: 0.75rem; }
    .info-row { display: flex; justify-content: space-between; padding: 0.35rem 0; }
    .info-label { color: #94a3b8; }
    .info-value { font-family: monospace; color: #e2e8f0; }
    ul { list-style: none; }
    ul li { padding: 0.25rem 0; }
    code {
      background: #0f172a;
      border: 1px solid #334155;
      border-radius: 6px;
      padding: 0.2rem 0.5rem;
      font-size: 0.875rem;
      color: #f97316;
    }
    .steps li { padding: 0.5rem 0; color: #cbd5e1; }
    .steps li strong { color: #e2e8f0; }
    .footer { text-align: center; color: #475569; font-size: 0.85rem; margin-top: 1.5rem; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="logo">vmsan</div>
      <div class="subtitle">Your microVM is running</div>
    </div>
    <div class="card">
      <h2>VM Info</h2>
      <div class="info-row">
        <span class="info-label">VM ID</span>
        <span class="info-value">${vmId}</span>
      </div>
      <div class="info-row">
        <span class="info-label">Runtime</span>
        <span class="info-value">node22-demo</span>
      </div>
      <div class="info-row">
        <span class="info-label">Published Ports</span>
        <span class="info-value">${ports.join(", ")}</span>
      </div>
    </div>
    <div class="card">
      <h2>Next Steps</h2>
      <ul class="steps">
        <li><strong>Connect to the VM:</strong> <code>vmsan connect ${vmId}</code></li>
        <li><strong>Deploy your app:</strong> Replace this page by stopping the welcome service and running your own server on the published port(s).</li>
        <li><strong>Stop this page:</strong> <code>systemctl stop vmsan-welcome</code></li>
      </ul>
    </div>
    <div class="footer">Powered by vmsan &middot; Firecracker microVMs</div>
  </div>
</body>
</html>`;
}

export function generateWelcomeServer(ports: number[]): string {
  const listeners = ports
    .map(
      (p) =>
        `server.listen(${p}, "0.0.0.0", () => console.log("vmsan-welcome listening on 0.0.0.0:${p}"));`,
    )
    .join("\n");

  return `"use strict";
const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");

const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf-8");

const server = http.createServer((req, res) => {
  res.writeHead(200, {
    "Content-Type": "text/html; charset=utf-8",
    "Cache-Control": "no-cache",
  });
  res.end(html);
});

${listeners}
`;
}

export function generateWelcomeService(ports: number[]): string {
  const description = `vmsan welcome page on port(s) ${ports.join(", ")}`;
  return `[Unit]
Description=${description}
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/node /opt/vmsan/welcome/server.js
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`;
}
