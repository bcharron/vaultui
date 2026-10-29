#!/bin/sh
# Fixture setup for the vaultui e2e test, per internal/tui/tui_test.go.
# Run against a fresh loopback dev vault:
#   vault server -dev -dev-root-token-id=root -dev-listen-address=127.0.0.1:8200 &
#   VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root scripts/dev-fixture.sh
set -e
export VAULT_ADDR=${VAULT_ADDR:-http://127.0.0.1:8200}
export VAULT_TOKEN=${VAULT_TOKEN:-root}

vault secrets enable -path=legacy -version=1 kv
vault kv put legacy/creds foo=bar baz=qux
vault kv put legacy/misc note=hello

# KV v2: apps/api with two versions
vault kv put secret/apps/api db_host=db01.internal
vault kv put secret/apps/api db_port=5433 notes=rotated\ port
vault kv put secret/apps/web owner=platform
vault kv put secret/db/config host=db01.internal

vault secrets enable transit
vault write -f transit/keys/aesguy type=aes256-gcm96
vault write -f transit/keys/eddy type=ed25519

vault secrets enable pki
vault write pki/roles/testrole allowed_domains=test.local allow_subdomains=true max_ttl=72h

cat > /tmp/team-apps.hcl <<'EOF'
# apps team: full control of the apps tree
path "secret/data/apps/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
EOF
vault policy write team-apps /tmp/team-apps.hcl

cat > /tmp/jwt-signer.hcl <<'EOF'
path "transit/sign/jwt-signer" {
  capabilities = ["update"]
}
EOF
vault policy write jwt-signer /tmp/jwt-signer.hcl

cat > /tmp/jwt-reader.hcl <<'EOF'
path "transit/verify/jwt-signer" {
  capabilities = ["update"]
}
EOF
vault policy write jwt-reader /tmp/jwt-reader.hcl

vault auth enable jwt
vault write auth/jwt/role/demo-service role_type=jwt \
  token_policies=jwt-signer,jwt-reader \
  bound_audiences=demo-services user_claim=sub ttl=2h
vault write auth/jwt/role/service-reader role_type=jwt \
  token_policies=jwt-reader \
  bound_audiences=demo-services user_claim=sub ttl=15m

# Sign a JWT with a throwaway key so a jwt login (and its lease) exists.
openssl genrsa -out /tmp/jwt-key.pem 2048 2>/dev/null
openssl rsa -in /tmp/jwt-key.pem -pubout -out /tmp/jwt-pub.pem 2>/dev/null
vault write auth/jwt/config jwt_validation_pubkeys=@/tmp/jwt-pub.pem
JWT=$(python3 - <<'PYEOF'
import base64, json, subprocess, time

def b64(b):
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()

header = b64(json.dumps({"alg": "RS256", "typ": "JWT"}).encode())
now = int(time.time())
payload = b64(json.dumps({"sub": "user-123", "aud": ["demo-services"], "iat": now, "exp": now + 3600}).encode())
sig = subprocess.run(["openssl", "dgst", "-sha256", "-sign", "/tmp/jwt-key.pem"],
                     input=(header + "." + payload).encode(), capture_output=True, check=True).stdout
print(header + "." + payload + "." + b64(sig))
PYEOF
)
vault write auth/jwt/login role=demo-service jwt="$JWT" >/dev/null

BOB_ID=$(vault write -format=json identity/entity name=bob \
  policies=jwt-signer,jwt-reader metadata=email=bob@example.com \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')
SVC_ID=$(vault write -format=json identity/entity name=service-a \
  policies=jwt-reader | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')
vault write identity/group name=apps-team type=internal \
  policies=jwt-signer member_entity_ids="$BOB_ID,$SVC_ID"

echo "fixture setup complete"
