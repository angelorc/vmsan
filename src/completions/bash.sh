# vmsan bash completion
# Add to ~/.bashrc:
#   eval "$(vmsan completion bash)"

_vmsan_get_vm_ids() {
  local vmsan_dir="${VMSAN_DIR:-$HOME/.vmsan}"
  local vms_dir="$vmsan_dir/vms"
  if [[ -d "$vms_dir" ]]; then
    for f in "$vms_dir"/*.json; do
      [[ -f "$f" ]] || continue
      local base="${f##*/}"
      printf '%s\n' "${base%.json}"
    done
  fi
}

_vmsan_complete() {
  local cur prev words cword
  _init_completion 2>/dev/null || {
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    words=("${COMP_WORDS[@]}")
    cword=$COMP_CWORD
  }

  local subcommands="create list ls start stop remove rm exec connect upload download network doctor completion"

  if [[ $cword -eq 1 ]]; then
    COMPREPLY=($(compgen -W "$subcommands" -- "$cur"))
    return
  fi

  local subcmd="${words[1]}"

  case "$prev" in
    --runtime)
      COMPREPLY=($(compgen -W "base node22 node24 python3.13" -- "$cur"))
      return
      ;;
    --network-policy)
      COMPREPLY=($(compgen -W "allow-all deny-all custom" -- "$cur"))
      return
      ;;
  esac

  local vm_ids
  vm_ids=$(_vmsan_get_vm_ids)

  case "$subcmd" in
    start)
      if [[ $cword -eq 2 && "$cur" != -* ]]; then
        COMPREPLY=($(compgen -W "$vm_ids" -- "$cur"))
        return
      fi
      ;;
    stop)
      if [[ "$cur" != -* ]]; then
        COMPREPLY=($(compgen -W "$vm_ids" -- "$cur"))
        return
      fi
      ;;
    remove|rm)
      if [[ "$cur" != -* ]]; then
        COMPREPLY=($(compgen -W "$vm_ids" -- "$cur"))
        return
      fi
      ;;
    exec|connect|upload|download|network)
      if [[ $cword -eq 2 && "$cur" != -* ]]; then
        COMPREPLY=($(compgen -W "$vm_ids" -- "$cur"))
        return
      fi
      ;;
    completion)
      if [[ $cword -eq 2 ]]; then
        COMPREPLY=($(compgen -W "bash zsh fish powershell" -- "$cur"))
        return
      fi
      ;;
  esac

  case "$subcmd" in
    create)
      COMPREPLY=($(compgen -W "--vcpus --memory --kernel --rootfs --from-image --runtime --project --disk --timeout --publish-port --snapshot --network-policy --allowed-domain --allowed-cidr --denied-cidr --no-seccomp --no-pid-ns --no-cgroup --no-netns --bandwidth --connect --silent --json --verbose" -- "$cur"))
      ;;
    list|ls|doctor)
      COMPREPLY=($(compgen -W "--json --verbose" -- "$cur"))
      ;;
    start|stop)
      COMPREPLY=($(compgen -W "--json --verbose" -- "$cur"))
      ;;
    remove|rm)
      COMPREPLY=($(compgen -W "--force -f --json --verbose" -- "$cur"))
      ;;
    exec)
      COMPREPLY=($(compgen -W "--sudo --interactive -i --no-extend-timeout --tty -t --workdir -w --env -e --json --verbose" -- "$cur"))
      ;;
    connect)
      COMPREPLY=($(compgen -W "--session -s --json --verbose" -- "$cur"))
      ;;
    upload)
      COMPREPLY=($(compgen -W "--dest -d --json --verbose" -- "$cur"))
      ;;
    download)
      COMPREPLY=($(compgen -W "--dest -d --json --verbose" -- "$cur"))
      ;;
    network)
      COMPREPLY=($(compgen -W "--network-policy --allowed-domain --allowed-cidr --denied-cidr --json --verbose" -- "$cur"))
      ;;
    completion)
      COMPREPLY=($(compgen -W "--json --verbose" -- "$cur"))
      ;;
  esac
}

complete -F _vmsan_complete vmsan
