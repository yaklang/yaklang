package utils

var BashCompleteScriptTpl = `#!/usr/bin/env {{.Shell}}
# ------------------------------------------------------------------------------
#          FILE:  {{.Path}}
#        AUTHOR:  inhere (https://github.com/inhere)
#       VERSION:  1.0.0
#   DESCRIPTION:  zsh shell complete for cli app: {{.BinName}}
# ------------------------------------------------------------------------------
# usage: source {{.Path}}
# run 'complete' to see registered complete function.
_complete_for_{{.BinName}} () {
    local cur prev
    _get_comp_words_by_ref -n = cur prev
    COMPREPLY=()
    commands="{{join .CmdNames " "}} help"
    case "$prev" in{{range $k,$v := .NameOpts}}
        {{$k}})
            COMPREPLY=($(compgen -W "{{$v}}" -- "$cur"))
            return 0
            ;;{{end}}
        help)
            COMPREPLY=($(compgen -W "$commands" -- "$cur"))
            return 0
            ;;
    esac
    COMPREPLY=($(compgen -W "$commands" -- "$cur"))
} &&
# complete -TaskFunc {auto_complete_func} {bin_filename}
# complete -TaskFunc _complete_for_{{.BinName}} -A file {{.BinName}} {{.BinName}}.exe
complete -TaskFunc _complete_for_{{.BinName}} {{.BinName}} {{.BinName}}.exe
`

var ZshCompleteScriptTpl = `#compdef {{.BinName}}
# ------------------------------------------------------------------------------
#          FILE:  {{.Path}}
#        AUTHOR:  inhere (https://github.com/inhere)
#       VERSION:  1.0.0
#   DESCRIPTION:  zsh shell complete for cli app: {{.BinName}}
# ------------------------------------------------------------------------------
# usage: source {{.Path}}
_complete_for_{{.BinName}} () {
    typeset -a commands
    commands+=({{range $k,$v := .NameDes}}
        '{{$k}}[{{$v}}]'{{end}}
        'help[Display help information]'
    )
    if (( CURRENT == 2 )); then
        # explain commands
        _values 'cliapp commands' ${commands[@]}
        return
    fi
    case ${words[2]} in{{range $k,$vs := .NameOpts}}
    {{$k}})
        _values 'command options' \{{range $vs}}
            {{.}}{{end}}
        ;;{{end}}
    help)
        _values "${commands[@]}"
        ;;
    *)
        # use files by default
        _files
        ;;
    esac
}
compdef _complete_for_{{.BinName}} {{.BinName}}
compdef _complete_for_{{.BinName}} {{.BinName}}.exe
`
