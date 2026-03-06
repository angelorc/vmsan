import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { isatty } from "node:tty";
import { consola } from "consola";
import { vmsanPaths } from "../paths.ts";
import { createCommandLogger } from "../lib/logger/index.ts";
import { handleCommandError } from "../errors/index.ts";
import { resolveVmState, waitForAgent } from "../lib/vm-context.ts";
import { AgentClient } from "../services/agent.ts";
import type { RunParams } from "../services/agent.ts";
import { ShellSession } from "../lib/shell/index.ts";
import { TimeoutExtender } from "../lib/timeout-extender.ts";

function shellEscape(s: string): string {
  if (/^[a-zA-Z0-9._\-/=:@]+$/.test(s)) return s;
  return "'" + s.replace(/'/g, "'\\''") + "'";
}

function parseEnvFlags(_targetCommand: string): Record<string, string> {
  const env: Record<string, string> = {};
  // Only scan argv tokens before the target command to avoid capturing
  // --env flags meant for the command being executed inside the VM.
  const argv = process.argv;
  let positionalCount = 0;

  for (let i = 2; i < argv.length; i++) {
    const arg = argv[i];

    if (arg === "--") break;

    // Track positionals: "exec" (subcommand), vmId, then target command.
    // Stop scanning once we hit the target command positional.
    if (!arg.startsWith("-")) {
      positionalCount++;
      // positional 1 = "exec", 2 = vmId, 3 = target command
      if (positionalCount >= 3) break;
      continue;
    }

    let value: string | undefined;

    if (arg === "--env" || arg === "-e") {
      value = argv[i + 1];
      if (value !== undefined) i++;
    } else if (arg.startsWith("--env=")) {
      value = arg.slice("--env=".length);
    } else if (arg.startsWith("-e=")) {
      value = arg.slice("-e=".length);
    }

    if (value !== undefined) {
      const eqIdx = value.indexOf("=");
      if (eqIdx > 0) {
        env[value.slice(0, eqIdx)] = value.slice(eqIdx + 1);
      }
    }
  }

  return env;
}

function buildVmsanPrompt(vmId: string): string {
  const shortId = vmId.slice(0, 8);
  return `\\[\\033[1;32m\\]vmsan:${shortId}\\[\\033[0m\\]:\\[\\033[1;34m\\]\\w\\[\\033[0m\\]\\$ `;
}

const execCommand = defineCommand({
  meta: {
    name: "exec",
    description: "Execute a command inside a running VM",
  },
  args: {
    vmId: {
      type: "positional",
      description: "VM ID, followed by command and arguments",
      required: true,
    },
    sudo: {
      type: "boolean",
      default: false,
      description: "Run with extended privileges (sudo)",
    },
    interactive: {
      type: "boolean",
      alias: "i",
      default: false,
      description: "Interactive shell mode (PTY)",
    },
    "no-extend-timeout": {
      type: "boolean",
      default: false,
      description: "Skip timeout extension (interactive only)",
    },
    tty: {
      type: "boolean",
      alias: "t",
      default: false,
      description: "Allocate a pseudo-TTY (accepted for compatibility)",
    },
    workdir: {
      type: "string",
      alias: "w",
      description: "Working directory inside the VM",
    },
    env: {
      type: "string",
      alias: "e",
      description: "Environment variable (KEY=VAL), repeatable",
    },
  },
  async run({ args }) {
    const cmdLog = createCommandLogger("exec");
    const paths = vmsanPaths();

    try {
      // Parse command and arguments from positionals
      const command = args._[1] as string | undefined;
      const commandArgs = args._.slice(2) as string[];

      if (!command) {
        consola.error("No command provided. Usage: vmsan exec <vm_id> <command> [...args]");
        cmdLog.emit();
        process.exitCode = 1;
        return;
      }

      // Validate --interactive requires a TTY
      if (args.interactive && !isatty(1)) {
        consola.error("--interactive requires a terminal (TTY).");
        process.exitCode = 1;
        return;
      }

      // Parse repeated --env / -e flags from process.argv (only CLI-owned tokens)
      const envVars = parseEnvFlags(command);

      const { state, guestIp, port, store } = resolveVmState(args.vmId, paths);
      consola.debug(`Agent endpoint: ${guestIp}:${port}`);

      consola.debug(`Waiting for agent on ${guestIp}:${port}...`);
      await waitForAgent(guestIp, port);

      if (args.interactive) {
        // Interactive mode: inject command into PTY shell
        const parts: string[] = [];

        // PS1 injection for custom vmsan prompt
        parts.push(`export PS1=${shellEscape(buildVmsanPrompt(args.vmId))} TERM=xterm-256color &&`);

        if (args.workdir) {
          parts.push(`cd ${shellEscape(args.workdir)} &&`);
        }

        for (const [key, val] of Object.entries(envVars)) {
          parts.push(`${key}=${shellEscape(val)}`);
        }

        parts.push(shellEscape(command));
        for (const a of commandArgs) {
          parts.push(shellEscape(a));
        }

        const injectedCmd = "clear; " + parts.join(" ") + "; exit $?\n";

        // Wire timeout extension
        let extender: TimeoutExtender | null = null;
        if (!args["no-extend-timeout"] && state.timeoutMs) {
          extender = new TimeoutExtender({
            vmId: args.vmId,
            store,
            paths,
          });
          extender.start();
        }

        try {
          const shell = new ShellSession({
            host: guestIp,
            port,
            token: state.agentToken,
            initialCommand: injectedCmd,
            user: args.sudo ? "root" : undefined,
          });

          await shell.connect();
        } finally {
          extender?.stop();
        }

        cmdLog.set({
          vmId: args.vmId,
          mode: "interactive",
          command,
          args: commandArgs,
          ...(args["no-extend-timeout"] && { noExtendTimeout: true }),
        });
        cmdLog.emit();
      } else {
        // Non-interactive mode: use Command class
        consola.debug(`exec: ${command} ${commandArgs.join(" ")}`);

        const agent = new AgentClient(`http://${guestIp}:${port}`, state.agentToken);

        const params: RunParams = {
          cmd: command,
          args: commandArgs.length > 0 ? commandArgs : undefined,
          cwd: args.workdir || undefined,
          env: Object.keys(envVars).length > 0 ? envVars : undefined,
          user: args.sudo ? "root" : undefined,
        };

        const ac = new AbortController();
        const onSignal = () => ac.abort();
        process.on("SIGINT", onSignal);
        process.on("SIGTERM", onSignal);

        try {
          const cmdObj = await agent.exec(params, {
            signal: ac.signal,
            onStdout: (line) => process.stdout.write(line + "\n"),
            onStderr: (line) => process.stderr.write(line + "\n"),
          });

          const result = await cmdObj.wait();
          process.exitCode = result.exitCode;
          if (result.timedOut) consola.error("Command timed out.");
        } catch (err) {
          if (!ac.signal.aborted) throw err;
          process.exitCode = 130;
        } finally {
          process.removeListener("SIGINT", onSignal);
          process.removeListener("SIGTERM", onSignal);
        }

        cmdLog.set({
          vmId: args.vmId,
          mode: "non-interactive",
          command,
          args: commandArgs,
        });
        cmdLog.emit();
      }
    } catch (error) {
      handleCommandError(error, cmdLog);
      process.exitCode = 1;
    }
  },
});

export default execCommand as CommandDef;
