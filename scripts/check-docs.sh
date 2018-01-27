#!/bin/bash

function cleanup() {
	git checkout Documentation
}

trap cleanup EXIT

make update-docs 2&>/dev/null

if [[ ! $(git diff --quiet Documentation) ]]; then 
	echo "Docs not correctly updated, please run \`make update-docs\`."
	exit 1
fi
