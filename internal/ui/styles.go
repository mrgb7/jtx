package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorTodo       = lipgloss.Color("#6B7280")
	colorInProgress = lipgloss.Color("#3B82F6")
	colorDone       = lipgloss.Color("#10B981")
	colorBlocked    = lipgloss.Color("#EF4444")
	colorReview     = lipgloss.Color("#8B5CF6")

	colorBug     = lipgloss.Color("#EF4444")
	colorStory   = lipgloss.Color("#10B981")
	colorTask    = lipgloss.Color("#3B82F6")
	colorEpic    = lipgloss.Color("#8B5CF6")
	colorSubtask = lipgloss.Color("#F59E0B")

	colorPriorityHigh     = lipgloss.Color("#EF4444")
	colorPriorityMedium   = lipgloss.Color("#F59E0B")
	colorPriorityLow      = lipgloss.Color("#6B7280")
	colorPriorityCritical = lipgloss.Color("#DC2626")

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FBBF24")).
			Background(lipgloss.Color("#111827")).
			Width(0). // set dynamically
			Padding(0, 1)

	detailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#9CA3AF")).
				Width(12)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24"))
)

func statusColor(category string) lipgloss.Color {
	switch category {
	case "indeterminate":
		return colorInProgress
	case "done":
		return colorDone
	case "new":
		return colorTodo
	case "Blocked":
		return colorBlocked
	}
	return colorTodo
}

func issueTypeColor(t string) lipgloss.Color {
	switch t {
	case "Bug":
		return colorBug
	case "Story":
		return colorStory
	case "Epic":
		return colorEpic
	case "Sub-task", "Subtask":
		return colorSubtask
	}
	return colorTask
}

func priorityColor(p string) lipgloss.Color {
	switch p {
	case "Critical", "Highest":
		return colorPriorityCritical
	case "High":
		return colorPriorityHigh
	case "Medium":
		return colorPriorityMedium
	case "Low", "Lowest", "Normal":
		return colorPriorityLow
	}
	return colorPriorityMedium
}

func issueTypeIcon(t string) string {
	switch t {
	case "Bug":
		return "B"
	case "Story":
		return "S"
	case "Epic":
		return "E"
	case "Sub-task", "Subtask":
		return "s"
	case "Task":
		return "T"
	}
	return "?"
}

func priorityIcon(p string) string {
	switch p {
	case "Critical", "Highest":
		return "!!"
	case "High":
		return "!"
	case "Medium":
		return "~"
	case "Low", "Lowest", "Normal":
		return "v"
	}
	return "-"
}
