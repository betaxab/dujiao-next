#!/bin/bash
BINPATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin:~/bin
export PATH="$BINPATH:$PATH"

GOBINPATH=/usr/local/go/bin
export PATH="$GOBINPATH:$PATH"

# export NVM_DIR="$HOME/.nvm"
# [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
# [ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion

# # pnpm
# export PNPM_HOME="$HOME/.local/share/pnpm"
# case ":$PATH:" in
#   *":$PNPM_HOME:"*) ;;
#   *) export PATH="$PNPM_HOME:$PATH" ;;
# esac
# # pnpm end

MODULE_PATH=$(go list -m)
BUILD_VERSION=v1.4.6-dev
BUILD_TYPE=source

test -d dist || mkdir -p dist
rm -f ./dist/dujiao-next
go mod tidy
CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags="-s -w \
    -X github.com/dujiao-next/internal/version.Version=$BUILD_VERSION \
    -X github.com/dujiao-next/internal/version.BuildType=$BUILD_TYPE" \
  -o dist/dujiao-next \
  ./cmd/server
if [ $? -ne 0 ]; then
	echo "error: build failed."
	exit 1
fi

test -e dist/dujiao-next && cp dist/dujiao-next /mnt/e/be/
