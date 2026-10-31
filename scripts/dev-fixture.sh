#!/bin/sh
# Fixture setup for the vaultui e2e test, per internal/tui/tui_test.go.
# Run against a fresh loopback dev vault:
#   vault server -dev -dev-root-token-id=root -dev-listen-address=127.0.0.1:8200 &
#   VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root scripts/dev-fixture.sh
# Beyond the e2e fixture this also enables every engine and auth method
# with a dedicated config endpoint (aws, gcp, azure, kubernetes, consul,
# nomad, rabbitmq, openldap, ssh; approle, cf, kerberos, radius) so the
# TUI's config rows can be exercised. Configs needing a live backing
# service (azure, kubernetes, openldap, ssh, cf, kerberos, radius,
# rabbitmq connections) stay unconfigured and show "(no data returned)".
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

# AWS: root and lease configs plus the roles the e2e test expects. The
# credentials are placeholders; skip_iam_verification keeps the write
# offline.
vault secrets enable aws
vault write aws/config/root access_key=AKIAFAKEEXAMPLE secret_key=placeholder \
  skip_iam_verification=true region=us-east-1
vault write aws/config/lease lease=5m lease_max=20m
vault write aws/roles/deploy credential_type=iam_user
vault write aws/roles/s3reader credential_type=assumed_role \
  role_arns=arn:aws:iam::123456789012:role/s3reader

# GCP: config with a throwaway service-account key so the config row
# has data to show.
vault secrets enable gcp
openssl genrsa -out /tmp/gcp-key.pem 2048 2>/dev/null
python3 - <<'PYEOF' > /tmp/gcp-sa.json
import json
sa = {
    "type": "service_account",
    "project_id": "dev-fixture",
    "private_key_id": "fakekeyid123",
    "private_key": open("/tmp/gcp-key.pem").read(),
    "client_email": "vault-dev@dev-fixture.iam.gserviceaccount.com",
    "client_id": "123456789",
}
print(json.dumps(sa))
PYEOF
vault write gcp/config credentials=@/tmp/gcp-sa.json

# The remaining config-endpoint engines. Consul and nomad tokens are
# dummies accepted without contacting the backing service; azure and
# kubernetes return defaults; the rest stay unconfigured.
vault secrets enable azure
vault secrets enable kubernetes
vault secrets enable consul
vault write consul/config/access token=dummy-consul-token
vault secrets enable nomad
vault write nomad/config/access token=dummy-nomad-token
vault write nomad/config/lease lease=5m lease_max=20m
vault secrets enable rabbitmq
vault write rabbitmq/config/lease lease=5m lease_max=20m
vault secrets enable openldap
vault secrets enable ssh

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

# Approle has no config endpoint but gets a role for the Roles screen;
# cf, kerberos and radius have config endpoints that need live backing
# services to configure, so they stay empty.
vault auth enable approle
vault write auth/approle/role/demo token_policies=team-apps token_ttl=30m
vault auth enable cf
vault auth enable kerberos
vault auth enable radius

BOB_ID=$(vault write -format=json identity/entity name=bob \
  policies=jwt-signer,jwt-reader metadata=email=bob@example.com \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')
SVC_ID=$(vault write -format=json identity/entity name=service-a \
  policies=jwt-reader | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')
vault write identity/group name=apps-team type=internal \
  policies=jwt-signer member_entity_ids="$BOB_ID,$SVC_ID"

echo "fixture setup complete"
