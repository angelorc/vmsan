# vmsan PowerShell completion
# Add to your $PROFILE:
#   Invoke-Expression (& vmsan completion powershell | Out-String)

function __vmsan_GetVmIds {
    $vmsanDir = if ($env:VMSAN_DIR) { $env:VMSAN_DIR } else { Join-Path $HOME '.vmsan' }
    $vmsDir = Join-Path $vmsanDir 'vms'
    if (Test-Path $vmsDir -PathType Container) {
        Get-ChildItem -Path $vmsDir -Filter '*.json' | ForEach-Object { $_.BaseName }
    }
}

Register-ArgumentCompleter -Native -CommandName vmsan -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $tokens = @($commandAst.CommandElements | ForEach-Object { $_.ToString() })
    $subcommands = @('create','list','ls','start','stop','remove','rm','exec','connect','upload','download','network','doctor','completion')

    $subcmd = $null
    $subcmdIdx = -1
    for ($i = 1; $i -lt $tokens.Count; $i++) {
        if ($subcommands -contains $tokens[$i]) {
            $subcmd = $tokens[$i]
            $subcmdIdx = $i
            break
        }
    }

    # Complete subcommand name
    if ($null -eq $subcmd) {
        $subcommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    # Enum value completions for flags with fixed choices
    $prevToken = if ($tokens.Count -ge 2) { $tokens[$tokens.Count - 2] } else { '' }
    switch ($prevToken) {
        '--runtime' {
            @('base','node22','node24','python3.13') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
            return
        }
        '--network-policy' {
            @('allow-all','deny-all','custom') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
            return
        }
    }

    $vmIds = @(__vmsan_GetVmIds)

    # Count positional (non-flag) tokens after the subcommand (excluding the current word)
    $positionals = 0
    for ($i = $subcmdIdx + 1; $i -lt $tokens.Count - 1; $i++) {
        if (-not $tokens[$i].StartsWith('-')) { $positionals++ }
    }

    # Subcommand-specific completions
    switch ($subcmd) {
        'start' {
            if ($positionals -eq 0 -and -not $wordToComplete.StartsWith('-')) {
                $vmIds | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', "VM ID: $_")
                }
                return
            }
        }
        { $_ -in @('stop','remove','rm') } {
            if (-not $wordToComplete.StartsWith('-')) {
                $vmIds | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', "VM ID: $_")
                }
                return
            }
        }
        { $_ -in @('exec','connect','upload','download','network') } {
            if ($positionals -eq 0 -and -not $wordToComplete.StartsWith('-')) {
                $vmIds | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', "VM ID: $_")
                }
                return
            }
        }
        'completion' {
            if (-not $wordToComplete.StartsWith('-')) {
                @('bash','zsh','fish','powershell') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
                return
            }
        }
    }

    # Flag completions per subcommand
    $flagMap = @{
        'create'     = @('--vcpus','--memory','--kernel','--rootfs','--from-image','--runtime','--project','--disk','--timeout','--publish-port','--snapshot','--network-policy','--allowed-domain','--allowed-cidr','--denied-cidr','--no-seccomp','--no-pid-ns','--no-cgroup','--no-netns','--bandwidth','--connect','--silent','--json','--verbose')
        'list'       = @('--json','--verbose')
        'ls'         = @('--json','--verbose')
        'start'      = @('--json','--verbose')
        'stop'       = @('--json','--verbose')
        'remove'     = @('--force','-f','--json','--verbose')
        'rm'         = @('--force','-f','--json','--verbose')
        'exec'       = @('--sudo','--interactive','-i','--no-extend-timeout','--tty','-t','--workdir','-w','--env','-e','--json','--verbose')
        'connect'    = @('--session','-s','--json','--verbose')
        'upload'     = @('--dest','-d','--json','--verbose')
        'download'   = @('--dest','-d','--json','--verbose')
        'network'    = @('--network-policy','--allowed-domain','--allowed-cidr','--denied-cidr','--json','--verbose')
        'doctor'     = @('--json','--verbose')
        'completion' = @('--json','--verbose')
    }

    if ($flagMap.ContainsKey($subcmd)) {
        $flagMap[$subcmd] | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)
        }
    }
}
