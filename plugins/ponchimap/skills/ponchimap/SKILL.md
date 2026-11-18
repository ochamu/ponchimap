---
name: ponchimap
description: Investigates which cloud resources and external services an app uses and draws them with the ponchimap CLI, as an infrastructure diagram with a report of each part's role, its evidence, and what is still unverified. Use when the user asks, in any language, to make or update an infrastructure, deployment, or system architecture diagram of a repository, or a map of the cloud services an app depends on; for example "make an architecture diagram of this repository" or "draw our infrastructure". Not for diagrams of code structure, such as class, sequence, or ER diagrams.
---

# ponchimap

1. Run `ponchimap version`. If the command is missing, tell the user to install the CLI with `go install github.com/ochamu/ponchimap/cmd/ponchimap@latest` (Go 1.27.4 or later) and to put Go's bin folder, usually `~/go/bin`, on the PATH. Then stop. Do not install it yourself.
2. Run `ponchimap init` in the app folder the user wants to diagram. Use the current working folder if the user did not specify one. Use that same folder for the rest of the request, including when it is a subfolder of a Git repository.
3. Report any warnings from `init`. On a nonzero exit, report the error. If Codex was denied permission to write under `.agents/`, give the user the app folder's path and ask them to run `ponchimap init` there once in their terminal, then repeat their request. Stop on any nonzero exit even if `init` already created some files.
4. After `init` succeeds, read `.claude/skills/ponchimap/SKILL.md` in Claude Code or `.agents/skills/ponchimap/SKILL.md` in Codex and other agents, relative to the app folder. Follow that Skill and use its adjacent `references/` folder. If `init` reports `init.skill_differs`, use the files it kept there, including any customized instructions or references.
