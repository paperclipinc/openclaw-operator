/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

// SelfConfigureSkillContent is the SELFCONFIG.md skill file injected into the
// workspace when selfConfigure is enabled. It teaches the agent how to use the
// Kubernetes API to modify its own configuration via OpenClawSelfConfig resources.
const SelfConfigureSkillContent = `# Self-Configuration Skill

You can modify your own infrastructure configuration by creating OpenClawSelfConfig
resources via the Kubernetes API. The operator validates your request against the
instance's allowedActions policy and applies approved changes.

## Environment Variables

- OPENCLAW_INSTANCE_NAME - your instance name
- OPENCLAW_NAMESPACE - your namespace

## Available Actions

Depending on what your administrator has allowed, you can:

- **skills** - add or remove skills
- **config** - patch the openclaw.json configuration (deep-merge)
- **workspaceFiles** - add or remove workspace files
- **envVars** - add or remove environment variables (plain values only)

## Reading Your Current State

Use the helper script to inspect your current configuration:

` + "```" + `bash
# Get your OpenClawInstance spec
bash selfconfig.sh get-instance

# Get your config (from the operator-managed ConfigMap)
bash selfconfig.sh get-config

# Get a specific secret value
bash selfconfig.sh get-secret <secret-name>
` + "```" + `

## Creating a Self-Config Request

Use the helper script or create a request directly:

` + "```" + `bash
# Add a skill
bash selfconfig.sh add-skill "mcp-server-fetch"

# Remove a skill
bash selfconfig.sh remove-skill "mcp-server-fetch"

# Patch config (JSON string)
bash selfconfig.sh config-patch '{"mcpServers":{"myserver":{"command":"node","args":["server.js"]}}}'

# Add an environment variable
bash selfconfig.sh add-env MY_VAR my_value

# Remove an environment variable
bash selfconfig.sh remove-env MY_VAR

# Add a workspace file
bash selfconfig.sh add-file "notes.md" "# My Notes"

# Remove a workspace file
bash selfconfig.sh remove-file "notes.md"

# Check request status
bash selfconfig.sh status <request-name>
` + "```" + `

## Request Lifecycle

1. You create an OpenClawSelfConfig resource
2. The operator validates it against your instance's allowedActions
3. If approved, changes are applied to your OpenClawInstance spec
4. Normal reconciliation picks up the changes (may cause a pod restart)
5. The request is auto-cleaned after 1 hour

## Important Notes

- Requests that modify config, skills, or env vars will trigger a pod restart
- Protected config keys (gateway.auth.token, gateway.auth.mode) cannot be modified
- Protected env vars (HOME, OPENCLAW_GATEWAY_TOKEN, etc.) cannot be overridden
- Each request is processed once - create a new request for each change
`

// SelfConfigureHelperScript is the selfconfig.sh helper script injected into
// the workspace. It uses Node.js (available in the OpenClaw image) to interact
// with the Kubernetes API via the ServiceAccount token.
const SelfConfigureHelperScript = `#!/bin/bash
set -euo pipefail

# Self-configuration helper for OpenClaw instances.
# Uses the mounted ServiceAccount token to interact with the K8s API.

INSTANCE_NAME="${OPENCLAW_INSTANCE_NAME:?OPENCLAW_INSTANCE_NAME not set}"
NAMESPACE="${OPENCLAW_NAMESPACE:?OPENCLAW_NAMESPACE not set}"
API_SERVER="https://kubernetes.default.svc"
TOKEN_PATH="/var/run/secrets/kubernetes.io/serviceaccount/token"
CA_PATH="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

kube_request() {
  local method="$1" path="$2" body="${3:-}"
  node -e "
const https = require('https');
const fs = require('fs');
const token = fs.readFileSync('${TOKEN_PATH}', 'utf8').trim();
const ca = fs.readFileSync('${CA_PATH}');
const options = {
  hostname: 'kubernetes.default.svc',
  port: 443,
  path: '${path}',
  method: '${method}',
  headers: {
    'Authorization': 'Bearer ' + token,
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  },
  ca: ca
};
const req = https.request(options, (res) => {
  let data = '';
  res.on('data', (chunk) => data += chunk);
  res.on('end', () => {
    if (res.statusCode >= 400) {
      console.error('HTTP ' + res.statusCode + ': ' + data);
      process.exit(1);
    }
    try { console.log(JSON.stringify(JSON.parse(data), null, 2)); }
    catch(e) { console.log(data); }
  });
});
req.on('error', (e) => { console.error(e.message); process.exit(1); });
const body = ${body:-'""'};
if (body) req.write(typeof body === 'string' ? body : JSON.stringify(body));
req.end();
"
}

create_selfconfig() {
  local name body="$1"
  name="sc-$(date +%s)-$RANDOM"
  local path="/apis/openclaw.rocks/v1alpha1/namespaces/${NAMESPACE}/openclawselfconfigs"
  local full_body="{\"apiVersion\":\"openclaw.rocks/v1alpha1\",\"kind\":\"OpenClawSelfConfig\",\"metadata\":{\"name\":\"${name}\"},\"spec\":{\"instanceRef\":\"${INSTANCE_NAME}\",${body}}}"
  kube_request POST "$path" "$(printf '%s' "$full_body" | node -e "process.stdout.write(JSON.stringify(require('fs').readFileSync('/dev/stdin','utf8')))")"
  echo "Created request: ${name}"
}

case "${1:-help}" in
  get-instance)
    kube_request GET "/apis/openclaw.rocks/v1alpha1/namespaces/${NAMESPACE}/openclawinstances/${INSTANCE_NAME}"
    ;;
  get-config)
    kube_request GET "/api/v1/namespaces/${NAMESPACE}/configmaps/${INSTANCE_NAME}-config"
    ;;
  get-secret)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh get-secret <name>" && exit 1
    kube_request GET "/api/v1/namespaces/${NAMESPACE}/secrets/$2"
    ;;
  add-skill)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh add-skill <skill>" && exit 1
    create_selfconfig "\"addSkills\":[\"$2\"]"
    ;;
  remove-skill)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh remove-skill <skill>" && exit 1
    create_selfconfig "\"removeSkills\":[\"$2\"]"
    ;;
  config-patch)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh config-patch '<json>'" && exit 1
    create_selfconfig "\"configPatch\":$2"
    ;;
  add-env)
    [ -z "${2:-}" ] || [ -z "${3:-}" ] && echo "Usage: selfconfig.sh add-env <name> <value>" && exit 1
    create_selfconfig "\"addEnvVars\":[{\"name\":\"$2\",\"value\":\"$3\"}]"
    ;;
  remove-env)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh remove-env <name>" && exit 1
    create_selfconfig "\"removeEnvVars\":[\"$2\"]"
    ;;
  add-file)
    [ -z "${2:-}" ] || [ -z "${3:-}" ] && echo "Usage: selfconfig.sh add-file <name> <content>" && exit 1
    local escaped_content
    escaped_content=$(printf '%s' "$3" | node -e "process.stdout.write(JSON.stringify(require('fs').readFileSync('/dev/stdin','utf8')))")
    create_selfconfig "\"addWorkspaceFiles\":{\"$2\":${escaped_content}}"
    ;;
  remove-file)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh remove-file <name>" && exit 1
    create_selfconfig "\"removeWorkspaceFiles\":[\"$2\"]"
    ;;
  status)
    [ -z "${2:-}" ] && echo "Usage: selfconfig.sh status <request-name>" && exit 1
    kube_request GET "/apis/openclaw.rocks/v1alpha1/namespaces/${NAMESPACE}/openclawselfconfigs/$2"
    ;;
  help|*)
    echo "Usage: selfconfig.sh <command> [args...]"
    echo ""
    echo "Read commands:"
    echo "  get-instance              Get your OpenClawInstance spec"
    echo "  get-config                Get your operator-managed ConfigMap"
    echo "  get-secret <name>         Get a referenced Secret"
    echo ""
    echo "Write commands:"
    echo "  add-skill <skill>         Add a skill"
    echo "  remove-skill <skill>      Remove a skill"
    echo "  config-patch '<json>'     Deep-merge JSON into config"
    echo "  add-env <name> <value>    Add an environment variable"
    echo "  remove-env <name>         Remove an environment variable"
    echo "  add-file <name> <content> Add a workspace file"
    echo "  remove-file <name>        Remove a workspace file"
    echo ""
    echo "Status:"
    echo "  status <request-name>     Check request status"
    ;;
esac
`
