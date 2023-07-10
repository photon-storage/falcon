#!/bin/bash

set -e
set -x

case "$(uname -s)" in
  Darwin)
    PLATFORM="MacOS"
    REPO=$(cd "$(dirname "$0")/../.."; pwd -P)
    ;;

  Linux)
    PLATFORM="Linux"
    SCRIPT_REALPATH=$(realpath -e -- "${BASH_SOURCE[0]}")
    REPO="${SCRIPT_REALPATH%/*}/../.."
    ;;

  CYGWIN*|MINGW32*|MSYS*|MINGW*)
    PLATFORM="Windows"
    echo "Windows NOT supported"
    exit 1
    ;;

  *)
    echo "unknown OS"
    exit 1
    ;;
esac

echo ${REPO}

PKG_0="github.com/ipfs/boxo"
PKG_1="github.com/ipfs/kubo"

# Default local photon-proto repo location.
LOCAL_REPO_0="${REPO}/../../ipfs/boxo"
LOCAL_REPO_1="${REPO}/../../ipfs/kubo"

if [ "$1" == "replace" ]; then
    go mod edit -replace $PKG_0=$LOCAL_REPO_0
    go mod edit -replace $PKG_1=$LOCAL_REPO_1
elif [ "$1" == "drop" ]; then
    go mod edit -dropreplace $PKG_0
    go mod edit -dropreplace $PKG_1
elif [ "$1" == "update" ]; then
    GOPROXY=direct go get -u $PKG_0
    GOPROXY=direct go get -u $PKG_1
else
    echo "unknown command"
fi
