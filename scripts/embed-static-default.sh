# embed-static-default.sh — Plan 10-8 default static embed linkage.
# Source after scripts/version.sh in build scripts.
#
# Default: STATIC_BUILD=true (libc-agnostic Linux embed).
# Opt out: STATIC_BUILD=false

if [ "${STATIC_BUILD}" = "false" ]; then
    EMBED_STATIC_BUILD=false
else
    EMBED_STATIC_BUILD=true
    STATIC_BUILD=true
fi
