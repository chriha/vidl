#!/bin/sh
# Tag the next release from the Conventional Commits since the last v* tag and push the tag; the release workflow
# (.github/workflows/release.yml) builds the GitHub release from it. Run via `make release`.
#
#   MAJOR=1     allow a major bump (v2+ needs a /v2 module path, or `go install …@latest` stops seeing releases)
#   V=vX.Y.Z    tag this version instead of the computed one (must be greater than the last tag)
#   DRY_RUN=1   print the version and the commits, then stop
#   SKIP_CI=1   skip the check that the ci workflow succeeded on HEAD
set -eu

die() {
	echo "version: $*" >&2
	exit 1
}

branch=$(git symbolic-ref --short -q HEAD || true)
[ "$branch" = main ] || die "not on main (on ${branch:-a detached HEAD})"
[ -z "$(git status --porcelain)" ] || die "working tree is not clean; commit or stash first"
git fetch -q --tags origin || die "git fetch origin failed"
head=$(git rev-parse HEAD)
[ "$head" = "$(git rev-parse origin/main)" ] || die "HEAD is not origin/main; pull or push first"

last=$(git describe --tags --abbrev=0 --match 'v[0-9]*' --exclude '*-*' 2>/dev/null || true) # pre-releases don't count
range=${last:+$last..}HEAD
count=$(git rev-list --count "$range")
[ "$count" -gt 0 ] || die "nothing to release since $last"

# split vX.Y.Z (a pre-release suffix is dropped) into $1 $2 $3
semver() {
	v=${1#v}
	echo "${v%%-*}" | tr . ' '
}

if [ -z "$last" ]; then
	next=v0.1.0 reason="first release"
	set -- 0 0 0
else
	# shellcheck disable=SC2046
	set -- $(semver "$last")
	if git log --format=%s "$range" | grep -Eq '^[a-z]+(\([^)]*\))?!:' ||
		git log --format=%B "$range" | grep -Eq '^BREAKING[ -]CHANGE:'; then
		next=v$(($1 + 1)).0.0 reason="breaking change"
	elif git log --format=%s "$range" | grep -Eq '^feat(\([^)]*\))?:'; then
		next=v$1.$(($2 + 1)).0 reason="new feature"
	else
		next=v$1.$2.$(($3 + 1)) reason="fixes and other changes only"
	fi
fi
lmaj=$1 lmin=$2 lpat=$3

if [ -n "${V:-}" ]; then
	echo "$V" | grep -Eqx 'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)' || die "V=$V is not a version vX.Y.Z"
	# shellcheck disable=SC2046
	set -- $(semver "$V")
	[ "$1" -gt "$lmaj" ] || { [ "$1" -eq "$lmaj" ] && { [ "$2" -gt "$lmin" ] ||
		{ [ "$2" -eq "$lmin" ] && [ "$3" -gt "$lpat" ]; }; }; } ||
		die "V=$V is not greater than the last release ${last:-v0.0.0}"
	next=$V reason="set by V"
fi

nmaj=$(semver "$next" | cut -d' ' -f1)
if [ "$nmaj" -gt "$lmaj" ] && [ "$nmaj" -ge 2 ] && [ "${MAJOR:-}" != 1 ]; then
	die "${last:-no release} -> $next is a major bump ($reason). A v2+ release needs a /v2 module path, or \`go install …@latest\` stops seeing releases. Pass MAJOR=1 to tag it anyway."
fi
! git rev-parse -q --verify "refs/tags/$next" >/dev/null || die "tag $next already exists"

if [ "${SKIP_CI:-}" != 1 ]; then
	command -v gh >/dev/null || die "gh not found: install it to check CI, or pass SKIP_CI=1"
	ci=$(gh run list --commit "$head" --workflow ci.yml --limit 1 --json status,conclusion \
		--jq '.[] | .status + " " + .conclusion') || die "gh run list failed (logged in?); or pass SKIP_CI=1"
	case $ci in
	"completed success") ;;
	"") die "no ci run for $head yet; wait for it, or pass SKIP_CI=1" ;;
	completed*) die "ci did not succeed on $head (${ci#completed })" ;;
	*) die "ci on $head has not finished ($ci); wait for it, or pass SKIP_CI=1" ;;
	esac
	# the release workflow pushes the Homebrew formula with this token; without it the tag would publish a half release
	gh secret list --json name --jq '.[].name' | grep -qx HOMEBREW_TAP_TOKEN ||
		die "repo secret HOMEBREW_TAP_TOKEN is not set (see README.md), or pass SKIP_CI=1"
fi

echo "next version: $next ($reason)"
if [ -n "$last" ]; then
	echo "commits since $last:"
	git log --oneline "$range"
else
	echo "$count commits, no earlier release"
fi
if [ "${DRY_RUN:-}" = 1 ]; then
	echo "dry run: no tag created"
	exit 0
fi

printf 'Tag and push %s? [y/N] ' "$next"
read -r answer || answer=
[ "$answer" = y ] || die "aborted; no tag created"
git tag -a "$next" -m "$next"
git push -q origin "$next" || {
	git tag -d "$next" >/dev/null
	die "pushing $next failed; local tag removed"
}
echo "pushed $next; the release workflow builds the GitHub release"
