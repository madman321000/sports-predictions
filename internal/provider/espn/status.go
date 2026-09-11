package espn

import "fmt"

func gameStatus(name, state string, completed bool) (string, error) {
	switch name {
	case "STATUS_POSTPONED":
		return "postponed", nil
	case "STATUS_CANCELED", "STATUS_CANCELLED":
		return "canceled", nil
	case "STATUS_SUSPENDED":
		return "suspended", nil
	case "STATUS_DELAYED":
		return "delayed", nil
	}
	if completed && state == "post" {
		return "final", nil
	}
	if state == "in" {
		return "in_progress", nil
	}
	if name == "STATUS_SCHEDULED" && state == "pre" {
		return "scheduled", nil
	}
	return "", fmt.Errorf("unsupported ESPN game status %q (%s)", name, state)
}
