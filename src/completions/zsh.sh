#compdef vmsan
# vmsan zsh completion
# Add to ~/.zshrc:
#   eval "$(vmsan completion zsh)"
# Or save directly to fpath:
#   vmsan completion zsh > "${fpath[1]}/_vmsan"

_vmsan_vm_ids() {
  local vmsan_dir="${VMSAN_DIR:-$HOME/.vmsan}"
  local vms_dir="$vmsan_dir/vms"
  local -a ids
  if [[ -d "$vms_dir" ]]; then
    for f in "$vms_dir"/*.json(N); do
      ids+=("${${f##*/}%.json}")
    done
  fi
  _describe 'VM ID' ids
}

_vmsan() {
  local context state state_descr line
  typeset -A opt_args

  local -a global_flags
  global_flags=(
    '--json[Output structured JSON]'
    '--verbose[Show detailed debug output]'
    '(- :)--version[Show version]'
    '(- :)--help[Show help]'
  )

  _arguments -C \
    $global_flags \
    '1:subcommand:->subcommand' \
    '*::args:->args'

  case $state in
    subcommand)
      local -a subcmds
      subcmds=(
        'create:Create and start a Firecracker microVM'
        'list:List all VMs'
        'ls:List all VMs (alias for list)'
        'start:Start a previously stopped VM'
        'stop:Stop one or more running VMs'
        'remove:Remove one or more VMs'
        'rm:Remove one or more VMs (alias for remove)'
        'exec:Execute a command inside a running VM'
        'connect:Connect to a running VM shell'
        'upload:Upload local files to a running VM'
        'download:Download a file from a running VM'
        'network:Update network policy on a running VM'
        'doctor:Check system prerequisites and installation health'
        'completion:Generate shell tab completion script'
      )
      _describe 'subcommand' subcmds
      ;;
    args)
      case $line[1] in
        create)
          _arguments \
            '--vcpus=[Number of vCPUs]:vcpus' \
            '--memory=[Memory in MiB]:memory' \
            '--kernel=[Path to kernel image]:kernel:_files' \
            '--rootfs=[Path to rootfs image]:rootfs:_files' \
            '--from-image=[Docker/OCI image (e.g. ubuntu:latest)]:image' \
            '--runtime=[Runtime environment]:runtime:(base node22 node24 python3.13)' \
            '--project=[Project label for grouping VMs]:project' \
            '--disk=[Root disk size (e.g. 10gb)]:disk' \
            '--timeout=[Auto-shutdown timeout (e.g. 30s, 5m, 1h)]:timeout' \
            '--publish-port=[Ports to forward (comma-separated)]:ports' \
            '--snapshot=[Snapshot ID to restore from]:snapshot' \
            '--network-policy=[Network mode]:policy:(allow-all deny-all custom)' \
            '--allowed-domain=[Domains to allow (comma-separated)]:domains' \
            '--allowed-cidr=[CIDRs to allow (comma-separated)]:cidrs' \
            '--denied-cidr=[CIDRs to deny (comma-separated)]:cidrs' \
            '--no-seccomp[Disable seccomp-bpf filter]' \
            '--no-pid-ns[Disable PID namespace isolation]' \
            '--no-cgroup[Disable cgroup resource limits]' \
            '--no-netns[Disable network namespace isolation]' \
            '--bandwidth=[Max bandwidth (e.g. 100mbit)]:bandwidth' \
            '--connect[Connect to VM shell after creation]' \
            '--silent[Suppress all output]' \
            $global_flags
          ;;
        list|ls|doctor)
          _arguments $global_flags
          ;;
        start)
          _arguments \
            '1:vmId:_vmsan_vm_ids' \
            $global_flags
          ;;
        stop)
          _arguments \
            '*:vmId:_vmsan_vm_ids' \
            $global_flags
          ;;
        remove|rm)
          _arguments \
            '(-f --force)'{-f,--force}'[Force removal of running VMs]' \
            '*:vmId:_vmsan_vm_ids' \
            $global_flags
          ;;
        exec)
          _arguments \
            '1:vmId:_vmsan_vm_ids' \
            '--sudo[Run with sudo]' \
            '(-i --interactive)'{-i,--interactive}'[Interactive PTY mode]' \
            '--no-extend-timeout[Skip timeout extension (interactive only)]' \
            '(-t --tty)'{-t,--tty}'[Allocate a pseudo-TTY]' \
            '(-w --workdir)'{-w,--workdir}'=[Working directory inside the VM]:workdir' \
            '(-e --env)'{-e,--env}'=[Environment variable KEY=VAL]:env' \
            $global_flags
          ;;
        connect)
          _arguments \
            '1:vmId:_vmsan_vm_ids' \
            '(-s --session)'{-s,--session}'=[Attach to existing session ID]:session' \
            $global_flags
          ;;
        upload)
          _arguments \
            '1:vmId:_vmsan_vm_ids' \
            '(-d --dest)'{-d,--dest}'=[Destination directory inside the VM]:dest' \
            '*:file:_files' \
            $global_flags
          ;;
        download)
          _arguments \
            '1:vmId:_vmsan_vm_ids' \
            '2:remote-path:' \
            '(-d --dest)'{-d,--dest}'=[Local destination path]:dest:_files' \
            $global_flags
          ;;
        network)
          _arguments \
            '1:vmId:_vmsan_vm_ids' \
            '--network-policy=[Network mode]:policy:(allow-all deny-all custom)' \
            '--allowed-domain=[Domains to allow (comma-separated)]:domains' \
            '--allowed-cidr=[CIDRs to allow (comma-separated)]:cidrs' \
            '--denied-cidr=[CIDRs to deny (comma-separated)]:cidrs' \
            $global_flags
          ;;
        completion)
          _arguments \
            '1:shell:(bash zsh fish powershell)' \
            $global_flags
          ;;
      esac
      ;;
  esac
}

compdef _vmsan vmsan
