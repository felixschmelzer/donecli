_ding() {
  local cur prev words cword
  _init_completion || return

  # Find the wrapped command (first non-flag arg after 'ding')
  local cmd_idx=1
  while [[ $cmd_idx -lt ${#COMP_WORDS[@]} && ${COMP_WORDS[$cmd_idx]} == -* ]]; do
    (( cmd_idx++ ))
  done

  if (( COMP_CWORD <= cmd_idx )); then
    # Complete ding's own flags or the command name
    if [[ $cur == -* ]]; then
      COMPREPLY=($(compgen -W '-h --help -v --version -c --config --completions' -- "$cur"))
    else
      COMPREPLY=($(compgen -c -- "$cur"))
    fi
    return
  fi

  # Delegate to the wrapped command's completion
  local cmd="${COMP_WORDS[$cmd_idx]}"
  local completion_func
  completion_func=$(complete -p "$cmd" 2>/dev/null | grep -oP '(?<=-F )\S+')

  if [[ -n "$completion_func" ]]; then
    COMP_WORDS=("${COMP_WORDS[@]:$cmd_idx}")
    (( COMP_CWORD -= cmd_idx ))
    "$completion_func"
  else
    # Fallback: file completion
    COMPREPLY=($(compgen -f -- "$cur"))
  fi
}
complete -F _ding ding
