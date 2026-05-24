_scrutineer() {
    local cur prev words cword

    if type _init_completion &>/dev/null; then
        _init_completion || return
    else
        COMPREPLY=()
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
        words=("${COMP_WORDS[@]}")
        cword=$COMP_CWORD
    fi

    local commands="review list show post completion help"

    if [[ $cword -eq 1 ]]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
        return
    fi

    local cmd="${words[1]}"

    case "$prev" in
        --mode)
            COMPREPLY=($(compgen -W "full commits both" -- "$cur"))
            return ;;
        --comments)
            COMPREPLY=($(compgen -W "summary inline both" -- "$cur"))
            return ;;
        --filter)
            COMPREPLY=($(compgen -W "all unreviewed reviewed" -- "$cur"))
            return ;;
        --agent)
            COMPREPLY=($(compgen -W "builtin claude codex gemini vibe opencode pi custom" -- "$cur"))
            return ;;
        --config)
            if type _filedir &>/dev/null; then
                _filedir 'toml'
            else
                COMPREPLY=($(compgen -f -X '!*.toml' -- "$cur"))
            fi
            return ;;
        --repo)
            if type _filedir &>/dev/null; then
                _filedir -d
            else
                COMPREPLY=($(compgen -d -- "$cur"))
            fi
            return ;;
        --mr)
            local ids
            ids=$(scrutineer list --format=ids 2>/dev/null)
            COMPREPLY=($(compgen -W "$ids" -- "$cur"))
            return ;;
    esac

    case "$cmd" in
        review)
            COMPREPLY=($(compgen -W "--project --mr --post --config --repo --batch --mode --comments --branch --commit --agent --model" -- "$cur"))
            ;;
        list)
            COMPREPLY=($(compgen -W "--project --config --repo --filter --format" -- "$cur"))
            ;;
        show)
            COMPREPLY=($(compgen -W "--project --config --mr --branch --commit" -- "$cur"))
            ;;
        post)
            COMPREPLY=($(compgen -W "--project --config --repo --mr --all --comments" -- "$cur"))
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"))
            ;;
    esac
}

complete -F _scrutineer scrutineer
