import { AgentClient } from "../../services/agent.ts";
import type { VmState } from "../vm-state.ts";

export function createAgentClient(state: VmState): AgentClient {
  const url = `http://${state.network.guestIp}:${state.agentPort}`;
  return new AgentClient(url, state.agentToken!);
}
