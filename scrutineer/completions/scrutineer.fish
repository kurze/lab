complete -c scrutineer -f

complete -c scrutineer -n __fish_use_subcommand -a review -d 'Review merge/pull requests or local branches'
complete -c scrutineer -n __fish_use_subcommand -a list -d 'List merge/pull requests and their review status'
complete -c scrutineer -n __fish_use_subcommand -a show -d 'Display stored review findings'
complete -c scrutineer -n __fish_use_subcommand -a post -d 'Post stored review findings to the forge'
complete -c scrutineer -n __fish_use_subcommand -a completion -d 'Generate shell completion scripts'
complete -c scrutineer -n __fish_use_subcommand -a help -d 'Show help'

# review
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l project -d 'Project path (owner/repo)'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l mr -d 'MR IDs (comma-separated)' -xa '(scrutineer list --format=ids 2>/dev/null)'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l post -d 'Post findings as comments'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l config -d 'Path to config file' -rF
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l repo -d 'Path to local repo clone' -ra '(__fish_complete_directories)'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l batch -d 'Review all unreviewed MRs'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l mode -d 'Review mode' -xa 'full commits both'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l comments -d 'Comment style' -xa 'summary inline both'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l branch -d 'Review a local branch'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l commit -d 'Review a single commit by SHA'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l agent -d 'Review agent' -xa 'builtin claude codex gemini vibe opencode pi custom'
complete -c scrutineer -n '__fish_seen_subcommand_from review' -l model -d 'LLM model'

# list
complete -c scrutineer -n '__fish_seen_subcommand_from list' -l project -d 'Project path'
complete -c scrutineer -n '__fish_seen_subcommand_from list' -l config -d 'Path to config file' -rF
complete -c scrutineer -n '__fish_seen_subcommand_from list' -l repo -d 'Path to local repo clone' -ra '(__fish_complete_directories)'
complete -c scrutineer -n '__fish_seen_subcommand_from list' -l filter -d 'Filter MRs' -xa 'all unreviewed reviewed'
complete -c scrutineer -n '__fish_seen_subcommand_from list' -l format -d 'Output format' -xa ids

# show
complete -c scrutineer -n '__fish_seen_subcommand_from show' -l project -d 'Project path'
complete -c scrutineer -n '__fish_seen_subcommand_from show' -l config -d 'Path to config file' -rF
complete -c scrutineer -n '__fish_seen_subcommand_from show' -l mr -d 'MR IDs' -xa '(scrutineer list --format=ids 2>/dev/null)'
complete -c scrutineer -n '__fish_seen_subcommand_from show' -l branch -d 'Branch name'
complete -c scrutineer -n '__fish_seen_subcommand_from show' -l commit -d 'Commit SHA'

# post
complete -c scrutineer -n '__fish_seen_subcommand_from post' -l project -d 'Project path'
complete -c scrutineer -n '__fish_seen_subcommand_from post' -l config -d 'Path to config file' -rF
complete -c scrutineer -n '__fish_seen_subcommand_from post' -l repo -d 'Path to local repo clone' -ra '(__fish_complete_directories)'
complete -c scrutineer -n '__fish_seen_subcommand_from post' -l mr -d 'MR IDs' -xa '(scrutineer list --format=ids 2>/dev/null)'
complete -c scrutineer -n '__fish_seen_subcommand_from post' -l all -d 'Post all stored MR results'
complete -c scrutineer -n '__fish_seen_subcommand_from post' -l comments -d 'Comment style' -xa 'summary inline both'

# completion
complete -c scrutineer -n '__fish_seen_subcommand_from completion' -xa 'bash zsh fish'
