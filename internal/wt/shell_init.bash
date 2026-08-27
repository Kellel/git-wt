cwt() {
    local destination task

    if [ "${1-}" = "new" ]; then
        shift
        task="${1-}"
        if [ -z "$task" ]; then
            printf '%s\n' 'usage: cwt new <task> [--branch <name>] [--base <ref>]' >&2
            return 2
        fi
        git wt new "$@" || return
        destination="$(git wt path "$task")" || return
    else
        destination="$(git wt path "$@")" || return
    fi

    cd -- "$destination"
}
