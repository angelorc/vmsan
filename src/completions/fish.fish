# vmsan fish completion
# Install permanently:
#   vmsan completion fish > ~/.config/fish/completions/vmsan.fish
# Or source temporarily in config.fish:
#   vmsan completion fish | source

function __vmsan_vm_ids
    set -l vmsan_dir (test -n "$VMSAN_DIR"; and echo "$VMSAN_DIR"; or echo "$HOME/.vmsan")
    set -l vms_dir "$vmsan_dir/vms"
    if test -d "$vms_dir"
        for f in $vms_dir/*.json
            test -f "$f"; or continue
            string replace -r '.*/([^/]+)\.json$' '$1' "$f"
        end
    end
end

function __vmsan_no_subcommand
    not __fish_seen_subcommand_from create list ls start stop remove rm exec connect upload download network doctor completion
end

# Disable file completion globally; re-enabled per subcommand where needed
complete -c vmsan -f

# Global flags
complete -c vmsan -l json    -d 'Output structured JSON'
complete -c vmsan -l verbose -d 'Show detailed debug output'

# Subcommands
complete -c vmsan -n __vmsan_no_subcommand -a create     -d 'Create and start a Firecracker microVM'
complete -c vmsan -n __vmsan_no_subcommand -a list       -d 'List all VMs'
complete -c vmsan -n __vmsan_no_subcommand -a ls         -d 'List all VMs (alias)'
complete -c vmsan -n __vmsan_no_subcommand -a start      -d 'Start a previously stopped VM'
complete -c vmsan -n __vmsan_no_subcommand -a stop       -d 'Stop one or more running VMs'
complete -c vmsan -n __vmsan_no_subcommand -a remove     -d 'Remove one or more VMs'
complete -c vmsan -n __vmsan_no_subcommand -a rm         -d 'Remove one or more VMs (alias)'
complete -c vmsan -n __vmsan_no_subcommand -a exec       -d 'Execute a command inside a running VM'
complete -c vmsan -n __vmsan_no_subcommand -a connect    -d 'Connect to a running VM shell'
complete -c vmsan -n __vmsan_no_subcommand -a upload     -d 'Upload local files to a running VM'
complete -c vmsan -n __vmsan_no_subcommand -a download   -d 'Download a file from a running VM'
complete -c vmsan -n __vmsan_no_subcommand -a network    -d 'Update network policy on a running VM'
complete -c vmsan -n __vmsan_no_subcommand -a doctor     -d 'Check system prerequisites and installation health'
complete -c vmsan -n __vmsan_no_subcommand -a completion -d 'Generate shell tab completion script'

# create
complete -c vmsan -n '__fish_seen_subcommand_from create' -l vcpus          -d 'Number of vCPUs' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l memory         -d 'Memory in MiB' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l kernel         -d 'Path to kernel image' -rF
complete -c vmsan -n '__fish_seen_subcommand_from create' -l rootfs         -d 'Path to rootfs image' -rF
complete -c vmsan -n '__fish_seen_subcommand_from create' -l from-image     -d 'Docker/OCI image (e.g. ubuntu:latest)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l runtime        -d 'Runtime environment' -r -a 'base node22 node24 python3.13'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l project        -d 'Project label for grouping VMs' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l disk           -d 'Root disk size (e.g. 10gb)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l timeout        -d 'Auto-shutdown timeout (e.g. 30s, 5m, 1h)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l publish-port   -d 'Ports to forward (comma-separated)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l snapshot       -d 'Snapshot ID to restore from' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l network-policy -d 'Network mode' -r -a 'allow-all deny-all custom'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l allowed-domain -d 'Domains to allow (comma-separated)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l allowed-cidr   -d 'CIDRs to allow (comma-separated)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l denied-cidr    -d 'CIDRs to deny (comma-separated)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l no-seccomp     -d 'Disable seccomp-bpf filter'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l no-pid-ns      -d 'Disable PID namespace isolation'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l no-cgroup      -d 'Disable cgroup resource limits'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l no-netns       -d 'Disable network namespace isolation'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l bandwidth      -d 'Max bandwidth (e.g. 100mbit)' -r
complete -c vmsan -n '__fish_seen_subcommand_from create' -l connect        -d 'Connect to VM shell after creation'
complete -c vmsan -n '__fish_seen_subcommand_from create' -l silent         -d 'Suppress all output'

# start / stop / remove / rm — VM ID completions
complete -c vmsan -n '__fish_seen_subcommand_from start'     -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from stop'      -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from remove rm' -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from remove rm' -s f -l force         -d 'Force removal of running VMs'

# exec
complete -c vmsan -n '__fish_seen_subcommand_from exec' -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from exec' -l sudo              -d 'Run with sudo'
complete -c vmsan -n '__fish_seen_subcommand_from exec' -s i -l interactive  -d 'Interactive PTY mode'
complete -c vmsan -n '__fish_seen_subcommand_from exec' -l no-extend-timeout -d 'Skip timeout extension (interactive only)'
complete -c vmsan -n '__fish_seen_subcommand_from exec' -s t -l tty          -d 'Allocate a pseudo-TTY'
complete -c vmsan -n '__fish_seen_subcommand_from exec' -s w -l workdir      -d 'Working directory inside the VM' -r
complete -c vmsan -n '__fish_seen_subcommand_from exec' -s e -l env          -d 'Environment variable KEY=VAL' -r

# connect
complete -c vmsan -n '__fish_seen_subcommand_from connect' -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from connect' -s s -l session   -d 'Attach to existing session ID' -r

# upload (re-enable file completion for local file args)
complete -c vmsan -n '__fish_seen_subcommand_from upload' -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from upload' -s d -l dest       -d 'Destination directory inside the VM' -r
complete -c vmsan -n '__fish_seen_subcommand_from upload' -F

# download
complete -c vmsan -n '__fish_seen_subcommand_from download' -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from download' -s d -l dest     -d 'Local destination path' -rF

# network
complete -c vmsan -n '__fish_seen_subcommand_from network' -a '(__vmsan_vm_ids)' -d 'VM ID'
complete -c vmsan -n '__fish_seen_subcommand_from network' -l network-policy -d 'Network mode' -r -a 'allow-all deny-all custom'
complete -c vmsan -n '__fish_seen_subcommand_from network' -l allowed-domain -d 'Domains to allow (comma-separated)' -r
complete -c vmsan -n '__fish_seen_subcommand_from network' -l allowed-cidr   -d 'CIDRs to allow (comma-separated)' -r
complete -c vmsan -n '__fish_seen_subcommand_from network' -l denied-cidr    -d 'CIDRs to deny (comma-separated)' -r

# completion
complete -c vmsan -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell' -d 'Shell'
