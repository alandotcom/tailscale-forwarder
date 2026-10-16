#!/bin/sh
set -eu

printf 'Go standard library\n\n'
cat "$(go env GOROOT)/LICENSE"

go list -mod=readonly -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' . |
    sort -u |
    while IFS='|' read -r module directory; do
        [ -n "$module" ] || continue
        printf '\n\nModule: %s\n' "$module"
        for notice in "$directory"/LICENSE* "$directory"/COPYING* "$directory"/NOTICE*; do
            [ -f "$notice" ] || continue
            printf '\n'
            cat "$notice"
        done
    done
