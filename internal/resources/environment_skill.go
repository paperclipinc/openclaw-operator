/*
Copyright 2026 OpenClaw.rocks

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

// EnvironmentSkillContent is the ENVIRONMENT.md file injected into every
// workspace. It tells the agent about its runtime environment - writable
// paths, package installation, and constraints - so it can make the best
// use of the container without trial-and-error.
const EnvironmentSkillContent = `# Runtime Environment

You run in a hardened Kubernetes pod. The root filesystem is read-only.

## Writable paths

| Path | Backed by | Persists across restarts |
|------|-----------|------------------------|
| ~/.openclaw/ | PVC | Yes |
| ~/.local/ | PVC | Yes |
| ~/.cache/ | PVC | Yes |
| ~/.config/ | PVC | Yes |
| /tmp | emptyDir | No |

Everything else is read-only. Do NOT attempt writing to system paths.

## OpenClaw CLI

The ` + "`openclaw`" + ` command is available on your PATH (symlinked to /app/openclaw.mjs).
Use it directly - you do NOT need to invoke node manually.

` + "```" + `bash
openclaw --help        # list commands
openclaw doctor        # run healthchecks
openclaw --version
` + "```" + `

## Installing packages

uv is pre-installed at ~/.local/bin/uv and already in your PATH.
~/.local/bin is in your PATH. All user-level installs persist across restarts.

### Python packages

` + "```" + `bash
# Install a Python package
pip install <package-name>
` + "```" + `

### Python CLI tools (isolated)

` + "```" + `bash
# Install a CLI tool (creates an isolated env, adds binary to PATH)
uv tool install <tool-name>
` + "```" + `

### Node.js (npm / npx)

npm global installs and npx are pre-configured to use writable paths.

` + "```" + `bash
# Install a global package
npm install -g <package-name>

# Run a one-off package
npx <package-name>
` + "```" + `

### Static binaries

` + "```" + `bash
curl -L <url> -o ~/.local/bin/<tool> && chmod +x ~/.local/bin/<tool>
` + "```" + `

## What does NOT work

- **apt-get / sudo / su** - no root access, the root filesystem is read-only
- **Writing to /usr, /etc, /var** - read-only system directories
- **HTTP downloads (port 80)** - blocked by network policy; use HTTPS (port 443) instead

If you need system-level packages (e.g. C libraries for compilation), ask the administrator
to provide a custom container image with those dependencies pre-installed.
`
