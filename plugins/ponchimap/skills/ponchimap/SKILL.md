---
name: ponchimap
description: Investigates which cloud resources and external services an app uses and draws them with the ponchimap CLI, as an infrastructure diagram with a report of each part's role, its evidence, and what is still unverified. Use when the user asks, in any language, to make or update an infrastructure, deployment, or system architecture diagram of a repository, or a map of the cloud services an app depends on; for example "make an architecture diagram of this repository" or "draw our infrastructure". Not for diagrams of code structure, such as class, sequence, or ER diagrams.
---

# ponchimap

The ponchimap plugin installs only this entry point. The steps for the diagram ship inside the ponchimap CLI, so that they always match the CLI that runs them. Get them and follow them:

1. Run `ponchimap version`. If the command is missing, tell the user to install the CLI with `go install github.com/ochamu/ponchimap/cmd/ponchimap@latest` (Go 1.27.4 or later) and to put Go's bin folder, usually `~/go/bin`, on the PATH. Then stop. Do not install it yourself.
2. In the repository's root folder, run `ponchimap init`. It places the Skill of this CLI and keeps what is already there. If it prints a warning, tell the user what it says.
3. Read the Skill that `init` placed: `.claude/skills/ponchimap/SKILL.md` in Claude Code, `.agents/skills/ponchimap/SKILL.md` in other agents. Follow it from its first step to its end for the user's request, as if the user had invoked it. Its references are in the `references/` folder beside it.
