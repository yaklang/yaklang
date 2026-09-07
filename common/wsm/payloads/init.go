package payloads

//go:generate gzip-embed -cache --base . --source ./behinder/static --source ./yakshell/static --source ./yakshell/encrypt --source ./godzilla/static --gz payloads.tar.gz --xor-key yaklang-payload-v1 --no-embed
