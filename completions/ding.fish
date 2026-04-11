# ding's own flags (only when no subcommand has been given yet)
complete -c ding -n '__fish_is_first_token' -s h -l help -d 'show help'
complete -c ding -n '__fish_is_first_token' -s v -l version -d 'print version'
complete -c ding -n '__fish_is_first_token' -s c -l config -d 'open interactive setup'
complete -c ding -n '__fish_is_first_token' -l completions -d 'output completion script' -a 'bash zsh fish'

# Complete executable names when no subcommand yet
complete -c ding -n '__fish_is_first_token' -a '(__fish_complete_subcommand)'

# Delegate to subcommand completions once a command is present
complete -c ding -n 'not __fish_is_first_token' -a '(__fish_complete_subcommand --fcs-skip=1)'
