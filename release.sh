#!/bin/bash
set -eo pipefail

readonly NAME="configurator"
readonly VERSION="$(cat release/VERSION)"
readonly RELEASES_ENDPOINT="https://api.github.com/repos/progrium/$NAME/releases"
readonly ASSETS_ENDPOINT="https://uploads.github.com/repos/progrium/$NAME/releases/%s/assets"

main() {
	local upload_url=$(curl -s --fail -d "{\"tag_name\": \"v$VERSION\"}" "$RELEASES_ENDPOINT?access_token=$GITHUB_ACCESS_TOKEN" | jq -r .upload_url | sed "s/[\{\}]//g")
	for file in release/*.tgz; do
		local name="$(basename $file)"
		echo "$name"
		curl --fail -X POST -H "Content-Type: application/gzip" --data-binary "@$file" "$upload_url=$name&access_token=$GITHUB_ACCESS_TOKEN" > /dev/null
	done
}

main