# jtx

A terminal UI for Jira. Navigate your board, view ticket details, and manage issues without leaving the terminal.

![Go](https://img.shields.io/badge/Go-1.21+-blue)

---

## Features

- Kanban board with color-coded status columns
- Ticket detail view with description, comments, and metadata
- Create, edit, transition, and assign tickets
- Search by JQL query or jump directly to a ticket by ID
- Configurable columns mapped to your Jira statuses

---

## Installation

```bash
git clone <repo>
cd jtx
go build -o jtx
mv jtx /usr/local/bin/  # or any directory in your PATH
```

---

## Configuration

### Required environment variables

| Variable        | Description                          |
|-----------------|--------------------------------------|
| `JIRA_API_TOKEN` | Atlassian API token                 |
| `JIRA_EMAIL`     | Atlassian account email             |
| `JIRA_URL`       | Jira base URL (e.g. `https://myorg.atlassian.net`) |

Generate an API token at: https://id.atlassian.com/manage-profile/security/api-tokens

### Config file

Default location: `$HOME/.config/jtx/config.toml` (or `config.yaml` / `config.yml`)

Override with: `jtx -config-dir /path/to/dir`

```toml
project = "SRE"   # Jira project key
jql     = ""      # Optional: custom JQL (overrides project default)

[[columns]]
title    = "To Do"
color    = "todo"
statuses = ["selected for development", "selected for dev", "open", "to do"]

[[columns]]
title    = "In Progress"
color    = "inprogress"
statuses = ["in progress"]

[[columns]]
title    = "Review"
color    = "review"
statuses = ["reviewing", "in review", "code review", "ready to deploy"]

[[columns]]
title    = "Done"
color    = "done"
statuses = ["done", "closed", "resolved", "released", "completed"]

```

`statuses` values are matched case-insensitively against each issue's status name.

#### Column color options

| Value       | Color  |
|-------------|--------|
| `todo`      | Gray   |
| `inprogress`| Blue   |
| `done`      | Green  |
| `review`    | Purple |
| `blocked`   | Red    |

### What issues are shown

Priority order (highest wins):

1. `-jql` CLI flag
2. `jql` field in config file
3. `project = KEY ORDER BY updated DESC` (if `project` is set)
4. `assignee = currentUser() ORDER BY updated DESC` (fallback)

---

## Usage

```bash
jtx                          # launch with config defaults
jtx -jql "sprint in openSprints() AND assignee = currentUser()"
jtx -config-dir ~/my-configs
```

### CLI flags

| Flag          | Description                                  |
|---------------|----------------------------------------------|
| `-jql`        | JQL query — overrides config and project key |
| `-config-dir` | Directory containing config file             |

---

## Keybindings

### Board

| Key                           | Action                      |
|-------------------------------|-----------------------------|
| `→` / `l` / `Tab`            | Next column                 |
| `←` / `h` / `Shift+Tab`      | Previous column             |
| `↓` / `j`                    | Next ticket                 |
| `↑` / `k`                    | Previous ticket             |
| `Enter`                       | Open ticket detail          |
| `o`                           | Open in browser             |
| `m`                           | Move ticket (transition)    |
| `n`                           | Create new ticket           |
| `s`                           | Search                      |
| `r`                           | Refresh                     |
| `q` / `Ctrl+C`               | Quit                        |

### Detail view

| Key                           | Action                      |
|-------------------------------|-----------------------------|
| `↑` / `↓`                    | Scroll                      |
| `m`                           | Move ticket (transition)    |
| `a`                           | Assign ticket               |
| `c`                           | Add comment                 |
| `e`                           | Edit description            |
| `t`                           | Edit title                  |
| `o`                           | Open in browser             |
| `n`                           | Create new ticket           |
| `s`                           | Search                      |
| `r`                           | Refresh                     |
| `q` / `Esc` / `Backspace`    | Back to board               |

### Search

| Key     | Action                                          |
|---------|-------------------------------------------------|
| `Enter` | Execute JQL query or jump to ticket (e.g. `OBS-123`) |
| `Esc`   | Cancel, return to board                         |

### Popups (comment / description editor)

| Key       | Action  |
|-----------|---------|
| `Ctrl+S`  | Save    |
| `Esc`     | Cancel  |

### Title editor

| Key     | Action  |
|---------|---------|
| `Enter` | Save    |
| `Esc`   | Cancel  |

### Create ticket wizard

The wizard has three steps: issue type → summary → description.

| Key       | Action                       |
|-----------|------------------------------|
| `↓` / `j` | Next option (type step)     |
| `↑` / `k` | Previous option (type step) |
| `Enter`   | Confirm / advance step      |
| `Ctrl+S`  | Submit (description step)   |
| `Esc`     | Back one step / cancel      |

### Transition / assign pickers

| Key       | Action          |
|-----------|-----------------|
| `↓` / `j` | Next item       |
| `↑` / `k` | Previous item   |
| `Enter`   | Apply           |
| `Esc` / `q` | Cancel        |

---

## Issue icons

### Type

| Icon | Type     |
|------|----------|
| `B`  | Bug      |
| `S`  | Story    |
| `E`  | Epic     |
| `s`  | Sub-task |
| `T`  | Task     |

### Priority

| Icon | Priority         |
|------|------------------|
| `!!` | Critical/Highest |
| `!`  | High             |
| `~`  | Medium           |
| `v`  | Low/Lowest       |
