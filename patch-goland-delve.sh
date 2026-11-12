#!/bin/sh
#
# This script is used to patch the GoLand dlv binary with a dlv version
# that has support for Undo.

usage() {
cat <<EOF
Usage: $0 [-u] <goland-base-path>
  -u	Uninstall patched dlv
EOF
exit
}

checkdeps() {
	deps="git go"
	for d in $deps
	do
		command -v "$d" >/dev/null || {
			echo "$0: missing dependency: $d" 1>&2
			exit 1
		}
	done
}

installdlv() {
	dst="$1"

	# Keep a copy of the original dlv binary so we can restore it if needed.
	if test -e "$dst/dlv" -a ! -e "$dst/dlv.orig"
	then
		cp "$dst/dlv" "$dst/dlv.orig"
	fi

	tmprepo=$(mktemp -d)
	trap 'GOPATH="$tmprepo" go clean -modcache 2>/dev/null || rm -rf $tmprepo' EXIT INT QUIT TERM HUP

	GOPATH="$tmprepo" go install github.com/undoio/delve/cmd/dlv@undo

	(
	install -m 755 "$tmprepo/bin/dlv" "$dst"
	) || exit 1

	echo "dlv has been installed to $dst"
}

uninstalldlv() {
	dst="$1"

	if test -e "$dst/dlv.orig"
	then
		mv "$dst/dlv.orig" "$dst/dlv"
		echo "dlv has been uninstalled from $dst"
	else
		echo "no patched dlv installation found"
	fi
}

uflag=0
for arg
do
case "$arg" in
--help|-h) usage ;;
--uninstall|-u) uflag=1 ;;
-*) echo "$0: unknown option $arg" ;;
*) dst="$arg" ;;
esac
done

if test -z "$dst"
then
	usage
fi

dst=$(readlink -e "$dst"plugins/go-plugin/lib/dlv/linux/)
if test ! $? -eq 0
then
	exit 1
fi

checkdeps

if test "$uflag" -eq 1
then
	uninstalldlv "$dst"
else
	installdlv "$dst"
fi
