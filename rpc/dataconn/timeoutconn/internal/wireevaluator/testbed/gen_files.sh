#!/usr/bin/env bash
set -e

cd "$( dirname "${BASH_SOURCE[0]}")"

FILESDIR="$(pwd)"/files

echo "[INFO] compile binary"
pushd .. >/dev/null
go build -o $FILESDIR/wireevaluator
popd >/dev/null

if [ ! -f "$FILESDIR/ssh_client_identity" ]; then
    echo "[INFO] gen ssh key"
    ssh-keygen -f "$FILESDIR/ssh_client_identity" -t ed25519
fi
