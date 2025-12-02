#!/usr/bin/env bash
# Generate test certificates for Cerberus mTLS testing

set -e

CERT_DIR="./certs/test"
mkdir -p "$CERT_DIR"

echo "Generating test certificates for Cerberus mTLS..."

# Generate CA
echo "1. Generating CA certificate..."
openssl req -x509 -new -nodes -sha256 -days 365 \
  -newkey rsa:2048 \
  -keyout "$CERT_DIR/ca.key" \
  -out "$CERT_DIR/ca.crt" \
  -subj "/CN=Tartarus Test CA" 2>/dev/null

# Generate server certificate
echo "2. Generating server certificate..."
openssl req -new -nodes \
  -newkey rsa:2048 \
  -keyout "$CERT_DIR/server.key" \
  -out "$CERT_DIR/server.csr" \
  -subj "/CN=localhost" 2>/dev/null

openssl x509 -req -sha256 -days 365 \
  -in "$CERT_DIR/server.csr" \
  -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
  -out "$CERT_DIR/server.crt" 2>/dev/null

# Generate agent/client certificate
echo "3. Generating agent client certificate..."
openssl req -new -nodes \
  -newkey rsa:2048 \
  -keyout "$CERT_DIR/agent.key" \
  -out "$CERT_DIR/agent.csr" \
  -subj "/CN=test-agent/O=agents" 2>/dev/null

# Create extension file for client auth
cat > "$CERT_DIR/client_ext.cnf" <<EOF
extendedKeyUsage=clientAuth
EOF

openssl x509 -req -sha256 -days 365 \
  -in "$CERT_DIR/agent.csr" \
  -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
  -extfile "$CERT_DIR/client_ext.cnf" \
  -out "$CERT_DIR/agent.crt" 2>/dev/null

# Clean up temporary files
rm -f "$CERT_DIR"/*.csr "$CERT_DIR"/*.srl "$CERT_DIR/client_ext.cnf"

echo ""
echo "✅ Test certificates generated successfully in $CERT_DIR/"
echo ""
echo "Files created:"
echo "  - ca.crt (CA certificate)"
echo "  - ca.key (CA private key)"
echo "  - server.crt (Server certificate)"
echo "  - server.key (Server private key)"
echo "  - agent.crt (Agent client certificate)"
echo "  - agent.key (Agent private key)"
echo ""
echo "To use with Olympus API:"
echo "  export TLS_CERT_FILE=\"$CERT_DIR/server.crt\""
echo "  export TLS_KEY_FILE=\"$CERT_DIR/server.key\""
echo "  export TLS_CA_FILE=\"$CERT_DIR/ca.crt\""
echo "  export TLS_CLIENT_AUTH=\"require-verify\""
echo ""
echo "To test with curl:"
echo "  curl --cert $CERT_DIR/agent.crt \\"
echo "       --key $CERT_DIR/agent.key \\"
echo "       --cacert $CERT_DIR/ca.crt \\"
echo "       https://localhost:8080/sandboxes"
