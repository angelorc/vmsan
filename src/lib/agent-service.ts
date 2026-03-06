/**
 * Template generators for the vmsan-agent systemd service.
 */

export function generateAgentService(): string {
  return `[Unit]
Description=Vmsan VM Agent

[Service]
Type=simple
ExecStart=/usr/local/bin/vmsan-agent
EnvironmentFile=/etc/vmsan/agent.env
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
`;
}

export function generateAgentEnv(token: string, port: number, vmId: string): string {
  return `VMSAN_AGENT_TOKEN=${token}
VMSAN_AGENT_PORT=${port}
VMSAN_VM_ID=${vmId}
VMSAN_DEFAULT_USER=ubuntu
`;
}
