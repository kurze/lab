#compdef scrutineer

_scrutineer() {
    local -a commands
    commands=(
        'review:Review merge/pull requests or local branches'
        'list:List merge/pull requests and their review status'
        'show:Display stored review findings'
        'post:Post stored review findings to the forge'
        'completion:Generate shell completion scripts'
        'help:Show help'
    )

    _arguments -C \
        '1:command:->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe 'command' commands
            ;;
        args)
            case $words[1] in
                review)
                    _arguments \
                        '--project[project path (owner/repo)]:project:' \
                        '--mr[MR IDs (comma-separated)]:mr:_scrutineer_mrs' \
                        '--post[post findings as comments]' \
                        '--config[path to config file]:config:_files -g "*.toml"' \
                        '--repo[path to local repo clone]:repo:_files -/' \
                        '--batch[review all unreviewed MRs]' \
                        '--mode[review mode]:mode:(full commits both)' \
                        '--comments[comment style]:style:(summary inline both)' \
                        '--branch[review a local branch]:branch:' \
                        '--commit[review a single commit by SHA]:commit:' \
                        '--agent[review agent]:agent:(builtin claude codex gemini vibe opencode pi custom)' \
                        '--model[LLM model]:model:'
                    ;;
                list)
                    _arguments \
                        '--project[project path]:project:' \
                        '--config[path to config file]:config:_files -g "*.toml"' \
                        '--repo[path to local repo clone]:repo:_files -/' \
                        '--filter[filter MRs]:filter:(all unreviewed reviewed)' \
                        '--format[output format]:format:(ids)'
                    ;;
                show)
                    _arguments \
                        '--project[project path]:project:' \
                        '--config[path to config file]:config:_files -g "*.toml"' \
                        '--mr[MR IDs]:mr:_scrutineer_mrs' \
                        '--branch[branch name]:branch:' \
                        '--commit[commit SHA]:commit:'
                    ;;
                post)
                    _arguments \
                        '--project[project path]:project:' \
                        '--config[path to config file]:config:_files -g "*.toml"' \
                        '--repo[path to local repo clone]:repo:_files -/' \
                        '--mr[MR IDs]:mr:_scrutineer_mrs' \
                        '--all[post all stored MR results]' \
                        '--comments[comment style]:style:(summary inline both)'
                    ;;
                completion)
                    _arguments '1:shell:(bash zsh fish)'
                    ;;
            esac
            ;;
    esac
}

_scrutineer_mrs() {
    local -a mrs
    mrs=(${(f)"$(scrutineer list --format=ids 2>/dev/null)"})
    [[ ${#mrs} -gt 0 ]] && _describe 'merge request' mrs
}

_scrutineer
