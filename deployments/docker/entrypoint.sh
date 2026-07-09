#!/bin/sh
set -eu

is_placeholder_secret() {
  case "$1" in
    "" | "change-this-password" | "replace-with-a-strong-password" | "change-me" | "replace-with-a-long-random-secret" | "replace-with-a-32-byte-random-key")
      return 0
      ;;
  esac
  return 1
}

if is_placeholder_secret "${ADMIN_PASSWORD:-}"; then
  echo "ERROR: ADMIN_PASSWORD must be set to a strong password, not an example value." >&2
  exit 1
fi

if [ "${#ADMIN_PASSWORD}" -lt 8 ]; then
  echo "ERROR: ADMIN_PASSWORD must be at least 8 characters long." >&2
  exit 1
fi

if is_placeholder_secret "${JWT_SECRET:-}"; then
  echo "ERROR: JWT_SECRET must be set to a strong random value, not an example value." >&2
  exit 1
fi

if [ "${PRODUCTION:-}" = "true" ]; then
  if is_placeholder_secret "${TOTP_ENCRYPTION_KEY:-}"; then
    echo "ERROR: TOTP_ENCRYPTION_KEY is required when PRODUCTION=true." >&2
    exit 1
  fi

  if [ "${#TOTP_ENCRYPTION_KEY}" -lt 32 ]; then
    echo "ERROR: TOTP_ENCRYPTION_KEY must be at least 32 characters long." >&2
    exit 1
  fi

  case "$TOTP_ENCRYPTION_KEY" in
    change-this-totp-encryption-key*)
      echo "ERROR: TOTP_ENCRYPTION_KEY cannot use the default weak prefix." >&2
      exit 1
      ;;
  esac
fi

mkdir -p /data

exec "$@"
