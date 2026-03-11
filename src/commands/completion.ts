import type { CommandDef } from "citty";
import { defineCommand } from "citty";
import { bashCompletionScript } from "../completions/bash.ts";
import { zshCompletionScript } from "../completions/zsh.ts";
import { fishCompletionScript } from "../completions/fish.ts";
import { powershellCompletionScript } from "../completions/powershell.ts";

const SUPPORTED_SHELLS = ["bash", "zsh", "fish", "powershell"] as const;

const completionCommand = defineCommand({
  meta: {
    name: "completion",
    description: "Generate shell tab completion script",
  },
  args: {
    shell: {
      type: "positional",
      description: "Shell to generate completion for (bash|zsh|fish|powershell)",
      required: true,
    },
  },
  run({ args }) {
    const shell = args.shell as string;

    if (!(SUPPORTED_SHELLS as readonly string[]).includes(shell)) {
      process.stderr.write(
        `Error: unsupported shell "${shell}". Supported: ${SUPPORTED_SHELLS.join(", ")}\n`,
      );
      process.exitCode = 1;
      return;
    }

    const scripts: Record<string, string> = {
      bash: bashCompletionScript,
      zsh: zshCompletionScript,
      fish: fishCompletionScript,
      powershell: powershellCompletionScript,
    };

    process.stdout.write(scripts[shell]!);
  },
});

export default completionCommand as CommandDef;
