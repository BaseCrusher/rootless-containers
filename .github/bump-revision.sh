#!/usr/bin/env bash
set -euo pipefail
dir="$1" dep="$2" type="${3:-}"
f="$dir/docker-bake.hcl"
[ -f "$f" ] || exit 0

# The variable annotated with this depName (empty for Dockerfile/plugins.json deps).
var=$(awk -v d="$dep" '/# renovate:/{hit=index($0,"depName="d) && $0 ~ ("depName="d"([^/[:alnum:]_-]|$)"); next}
                       hit && /variable/{gsub(/"/,"",$2); print $2; exit}' "$f")

# Packaged <version> (the variable used in the tags) already changes the tag — no revision bump.
if [ -n "$var" ] && grep -q "\${$var}" <(awk '/tags[[:space:]]*=/{t=1} t{print} /]/{t=0}' "$f"); then
  exit 0
fi

cur=$(awk '/variable "IMAGE_REVISION"/{f=1} f&&/default/{gsub(/"/,"",$3); print $3; exit}' "$f")
if [ "$type" = major ]; then
  next=$(awk -F. -v OFS=. '{print $1+1, 0}' <<<"$cur")
else
  next=$(awk -F. -v OFS=. '{$2++; print}' <<<"$cur")
fi
awk -v c="$cur" -v n="$next" '!done && $0 ~ ("default = \""c"\"") {sub("\""c"\"", "\""n"\""); done=1} 1' "$f" >"$f.tmp" && mv "$f.tmp" "$f"
