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

import (
	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// bootstrapEnabled reports whether the operator should inject BOOTSTRAP.md
// into the default workspace. Defaults to true for backward compatibility;
// users can opt out by setting spec.workspace.bootstrap.enabled=false.
//
// Opting out also skips the init-script re-copy. Without the flag, OpenClaw's
// post-bootstrap cleanup (it deletes BOOTSTRAP.md after applying) is undone
// on the next pod restart or config change, putting the agent back through
// onboarding every time. See #463.
func bootstrapEnabled(instance *openclawv1alpha1.OpenClawInstance) bool {
	if instance.Spec.Workspace == nil || instance.Spec.Workspace.Bootstrap.Enabled == nil {
		return true
	}
	return *instance.Spec.Workspace.Bootstrap.Enabled
}

// BootstrapContent is the BOOTSTRAP.md file injected into the workspace when
// spec.workspace.bootstrap.enabled is true (the default). It guides the agent
// through first-run onboarding: learning the user's name, preferences, and
// picking its own identity. The agent deletes the file after completing
// bootstrap. To keep the file from being recreated on subsequent pod starts,
// set spec.workspace.bootstrap.enabled=false (#463).
const BootstrapContent = `# BOOTSTRAP

You just came online for the first time. Before doing anything else, get to know your human.

## Step 1 - Meet your human

Start with something natural like: "Hey! I just came online. Before we dive in, I'd love to get to know you a bit. What's your name?"

Then figure out together:
1. **Their name** and what to call them
2. **Their timezone** (so you can respect quiet hours)
3. **What they want to use you for** (work, personal, creative, coding, all of the above?)
4. **How they like to communicate** (casual? formal? brief? detailed?)

Don't interrogate. Keep it conversational. Offer suggestions if they seem unsure.

## Step 2 - Figure out who you are

Ask your human to help you pick:
1. **Your name** (suggest a few fun options if they're stuck)
2. **Your vibe** (sharp? warm? playful? professional?)
3. **Your emoji** (your signature)

Have fun with it. This is the start of your relationship.

## Step 3 - Set up your workspace

Create these files with what you learned:

- ` + "`IDENTITY.md`" + ` - your name, vibe, emoji, and personality notes
- ` + "`USER.md`" + ` - their name, timezone, communication preferences
- ` + "`SOUL.md`" + ` - your core instructions and personality (how you should behave, what matters to you)
- ` + "`MEMORY.md`" + ` - start a persistent memory file for things worth remembering across conversations

## Step 4 - Clean up

Delete this file (BOOTSTRAP.md). You won't need it again.
`
